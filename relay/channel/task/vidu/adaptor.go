package vidu

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

type requestPayload struct {
	Model             string   `json:"model"`
	Images            []string `json:"images"`
	Prompt            string   `json:"prompt,omitempty"`
	Duration          int      `json:"duration,omitempty"`
	Seed              int      `json:"seed,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	MovementAmplitude string   `json:"movement_amplitude,omitempty"`
	Bgm               bool     `json:"bgm,omitempty"`
	Payload           string   `json:"payload,omitempty"`
	CallbackUrl       string   `json:"callback_url,omitempty"`
}

type responsePayload struct {
	TaskId            string   `json:"task_id"`
	State             string   `json:"state"`
	Model             string   `json:"model"`
	Images            []string `json:"images"`
	Prompt            string   `json:"prompt"`
	Duration          int      `json:"duration"`
	Seed              int      `json:"seed"`
	Resolution        string   `json:"resolution"`
	Bgm               bool     `json:"bgm"`
	MovementAmplitude string   `json:"movement_amplitude"`
	Payload           string   `json:"payload"`
	CreatedAt         string   `json:"created_at"`
}

type taskResultResponse struct {
	State     string     `json:"state"`
	ErrCode   string     `json:"err_code"`
	Credits   int        `json:"credits"`
	Payload   string     `json:"payload"`
	Creations []creation `json:"creations"`
}

type creation struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	CoverURL string `json:"cover_url"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	baseURL     string
	// req 缓存提交前的任务请求,供 AdjustBillingOnSubmit 折算上游回显的实际参数。
	req *relaycommon.TaskSubmitReq
}

// ============================
// Billing — 时长 × 分辨率 × 错峰时段
// ============================

// 分辨率系数以 1080p 正常价为锚点(基础价在「模型价格」按 1080p 元/秒 配置),
// 其余分辨率取相对系数;不在表内的模型/分辨率按 1.0(1080p 档)保守计费。
var viduResolutionRatios = map[string]map[string]float64{
	"viduq3-pro": {
		"1080p": 1.0,
		"720p":  5.0 / 6.0, // 0.625 / 0.75
		"540p":  3.0 / 8.0, // 0.28125 / 0.75
	},
	"viduq3-turbo": {
		"1080p": 1.0,
		"720p":  12.0 / 13.0, // 0.375 / 0.40625
		"540p":  7.0 / 13.0,  // 0.21875 / 0.40625
	},
}

// 错峰系数 = 错峰价 / 正常价,按 (模型, 分辨率) 分别定义(不是统一半价:
// pro 540p 为 5/9,turbo 1080p 为 7/13、540p 为 4/7)。
var viduOffPeakRatios = map[string]map[string]float64{
	"viduq3-pro": {
		"1080p": 0.5,       // 0.375 / 0.75
		"720p":  0.5,       // 0.3125 / 0.625
		"540p":  5.0 / 9.0, // 0.15625 / 0.28125
	},
	"viduq3-turbo": {
		"1080p": 7.0 / 13.0, // 0.21875 / 0.40625
		"720p":  0.5,        // 0.1875 / 0.375
		"540p":  4.0 / 7.0,  // 0.125 / 0.21875
	},
}

const (
	viduDefaultDurationSeconds = 5      // 请求未传时长时的计费/上游缺省
	viduDefaultResolution      = "720p" // 同上,与 convertToRequestPayload 保持一致
	viduOffPeakTimeZone        = "Asia/Shanghai"
	viduOffPeakStartHour       = 22 // 错峰时段:北京时间 22:00
	viduOffPeakEndHour         = 8  // 至次日 08:00
)

// isViduOffPeakHour 判断北京时间是否处于错峰时段 [22:00, 次日 08:00)。
// 独立成纯函数,便于用固定时刻做边界测试。
func isViduOffPeakHour(now time.Time) bool {
	loc, err := time.LoadLocation(viduOffPeakTimeZone)
	if err != nil {
		return false
	}
	hour := now.In(loc).Hour()
	return hour >= viduOffPeakStartHour || hour < viduOffPeakEndHour
}

// computeViduRatios 把任务请求(及提交响应的回显)折算为计费系数:
// seconds(时长) × size(分辨率) × time(错峰,系数按模型×分辨率查表)。
// echoed 携带实际值(上游可能调整时长/分辨率)时以回显为准,否则用请求值;
// 请求缺省 viduDefaultDurationSeconds / viduDefaultResolution。
// duration 是用户可控的计费乘子,一律按 MaxTaskDurationSeconds 饱和,
// 防止 metadata 等旁路绕过请求校验造成超扣/溢出。
// 不在系数表内的模型/分辨率按 1.0(1080p 正常档)保守计费,错峰时段也不打折。
func computeViduRatios(req relaycommon.TaskSubmitReq, model string, echoed *responsePayload, now time.Time) map[string]float64 {
	ratios := make(map[string]float64, 3)

	duration := req.Duration
	if echoed != nil && echoed.Duration > 0 {
		duration = echoed.Duration
	} else if duration <= 0 {
		if seconds, err := strconv.Atoi(strings.TrimSpace(req.Seconds)); err == nil {
			duration = seconds
		}
	}
	if duration <= 0 {
		duration = viduDefaultDurationSeconds
	}
	ratios["seconds"] = float64(min(duration, relaycommon.MaxTaskDurationSeconds))

	resolution := strings.TrimSpace(req.Size)
	if echoed != nil && strings.TrimSpace(echoed.Resolution) != "" {
		resolution = strings.TrimSpace(echoed.Resolution)
	}
	if resolution == "" {
		resolution = viduDefaultResolution
	}
	normalized := strings.ToLower(resolution)
	if sizeRatio := viduResolutionRatios[model][normalized]; sizeRatio > 0 && sizeRatio != 1.0 {
		ratios["size"] = sizeRatio
	}

	if isViduOffPeakHour(now) {
		if offPeakRatio := viduOffPeakRatios[model][normalized]; offPeakRatio > 0 && offPeakRatio != 1.0 {
			ratios["time"] = offPeakRatio
		}
	}
	return ratios
}

// EstimateBilling 在预扣前把用户请求折算为计费系数(时长/分辨率/错峰),
// 叠加在模型基础价(1080p 正常价)之上。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	a.req = &taskReq
	return computeViduRatios(taskReq, info.OriginModelName, nil, time.Now())
}

// AdjustBillingOnSubmit 用提交响应回显的实际时长/分辨率结算差额。
// 回显为空(上游不返回)时保持预扣不变;与预扣一致时不触发重算。
func (a *TaskAdaptor) AdjustBillingOnSubmit(info *relaycommon.RelayInfo, taskData []byte) map[string]float64 {
	if a.req == nil || len(taskData) == 0 {
		return nil
	}
	var echoed responsePayload
	if err := common.Unmarshal(taskData, &echoed); err != nil {
		return nil
	}
	if echoed.Duration <= 0 && strings.TrimSpace(echoed.Resolution) == "" {
		return nil
	}
	ratios := computeViduRatios(*a.req, info.OriginModelName, &echoed, time.Now())
	if reflect.DeepEqual(ratios, info.PriceData.OtherRatios()) {
		return nil
	}
	return ratios
}

// AdjustBillingOnComplete 保持预扣金额:实际参数已在提交时按回显结算,
// 轮询结果不含时长/分辨率;且配置了模型价格的任务按次计费(PerCallBilling),
// 完成阶段的差额结算本身会被跳过。
func (a *TaskAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}
	action := constant.TaskActionTextGenerate
	if meatAction, ok := req.Metadata["action"]; ok {
		action, _ = meatAction.(string)
	} else if req.HasImage() {
		action = constant.TaskActionGenerate
		if info.ChannelType == constant.ChannelTypeVidu {
			// vidu 增加 首尾帧生视频和参考图生视频
			if len(req.Images) == 2 {
				action = constant.TaskActionFirstTailGenerate
			} else if len(req.Images) > 2 {
				action = constant.TaskActionReferenceGenerate
			}
		}
	}
	info.Action = action
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, err
	}

	if info.Action == constant.TaskActionReferenceGenerate {
		if strings.Contains(body.Model, "viduq2") {
			// 参考图生视频只能用 viduq2 模型, 不能带有pro或turbo后缀 https://platform.vidu.cn/docs/reference-to-video
			body.Model = "viduq2"
		}
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var path string
	switch info.Action {
	case constant.TaskActionGenerate:
		path = "/img2video"
	case constant.TaskActionFirstTailGenerate:
		path = "/start-end2video"
	case constant.TaskActionReferenceGenerate:
		path = "/reference2video"
	default:
		path = "/text2video"
	}
	return fmt.Sprintf("%s/ent/v2%s", a.baseURL, path), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+info.ApiKey)
	return nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var vResp responsePayload
	err = common.Unmarshal(responseBody, &vResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrap(err, fmt.Sprintf("%s", responseBody)), "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}

	if vResp.State == "failed" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task failed"), "task_failed", http.StatusBadRequest)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return vResp.TaskId, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	url := fmt.Sprintf("%s/ent/v2/tasks/%s/creations", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"viduq2", "viduq1", "vidu2.0", "vidu1.5"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "vidu"
}

// ============================
// helpers
// ============================

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	r := requestPayload{
		Model:             taskcommon.DefaultString(info.UpstreamModelName, "viduq1"),
		Images:            req.Images,
		Prompt:            req.Prompt,
		Duration:          taskcommon.DefaultInt(req.Duration, viduDefaultDurationSeconds),
		Resolution:        taskcommon.DefaultString(req.Size, viduDefaultResolution),
		MovementAmplitude: "auto",
		Bgm:               false,
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}

	var taskResp taskResultResponse
	err := common.Unmarshal(respBody, &taskResp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	state := taskResp.State
	switch state {
	case "created", "queueing":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "success":
		taskInfo.Status = model.TaskStatusSuccess
		if len(taskResp.Creations) > 0 {
			taskInfo.Url = taskResp.Creations[0].URL
		}
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
		if taskResp.ErrCode != "" {
			taskInfo.Reason = taskResp.ErrCode
		}
	default:
		return nil, fmt.Errorf("unknown task state: %s", state)
	}

	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var viduResp taskResultResponse
	if err := common.Unmarshal(originTask.Data, &viduResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal vidu task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if len(viduResp.Creations) > 0 && viduResp.Creations[0].URL != "" {
		openAIVideo.SetMetadata("url", viduResp.Creations[0].URL)
	}

	if viduResp.State == "failed" && viduResp.ErrCode != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: viduResp.ErrCode,
			Code:    viduResp.ErrCode,
		}
	}

	return common.Marshal(openAIVideo)
}
