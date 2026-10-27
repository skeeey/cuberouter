package doubao

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ============================
// Request / Response structures
// ============================

// ContentItem、MediaURL 与网关统一的 Ark 内容项类型一致，
// 客户端直传的 content 数组可零拷贝透传。
type ContentItem = relaycommon.TaskContentItem

type MediaURL = relaycommon.TaskMediaURL

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier string         `json:"safety_identifier,omitempty"`
	Priority         *dto.IntValue  `json:"priority,omitempty"`
	Resolution       string         `json:"resolution,omitempty"`
	Ratio            string         `json:"ratio,omitempty"`
	Duration         *dto.IntValue  `json:"duration,omitempty"`
	Frames           *dto.IntValue  `json:"frames,omitempty"`
	Seed             *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed      *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark        *dto.BoolValue `json:"watermark,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ============================
// Billing facts
// ============================

// init 注册 Seedance 标准版的计费事实 schema：tiered_expr 保存时按此校验
// u() 键，并生成冒烟测试向量。2.5 不支持 4K，2.0 标准版支持 4K；fast/mini
// 的单价不同，不在此 schema 范围内。
func init() {
	billingexpr.RegisterTaskUsageSchema(isSeedance25Model, map[string]billingexpr.UsageFieldSchema{
		"tokens":      {Type: "number", Unit: "token"},
		"resolution":  {Type: "string", Enum: []string{"480p", "720p", "1080p"}},
		"video_input": {Type: "string", Enum: []string{"none", "video"}},
		"unit_price":  {Type: "number", Unit: "USD/1M tokens"},
	})
	billingexpr.RegisterTaskUsageSchema(isSeedance20Model, map[string]billingexpr.UsageFieldSchema{
		"tokens":      {Type: "number", Unit: "token"},
		"resolution":  {Type: "string", Enum: []string{"480p", "720p", "1080p", "4k"}},
		"video_input": {Type: "string", Enum: []string{"none", "video"}},
		"unit_price":  {Type: "number", Unit: "USD/1M tokens"},
	})
}

type seedanceRequestSettings struct {
	Resolution string
	Duration   int
	HasVideo   bool
}

// deriveSeedanceRequestSettings 统一推导请求的生效分辨率/时长/是否含视频输入，
// 成形载荷与计费事实共用同一份结果，避免发给上游的档位与计费读取的档位漂移。
func deriveSeedanceRequestSettings(req *relaycommon.TaskSubmitReq) (seedanceRequestSettings, error) {
	var meta struct {
		Resolution string        `json:"resolution,omitempty"`
		Duration   *dto.IntValue `json:"duration,omitempty"`
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &meta); err != nil {
		return seedanceRequestSettings{}, errors.Wrap(err, "unmarshal metadata failed")
	}

	resolution := meta.Resolution
	if resolution == "" {
		resolution = req.Resolution
	}
	if resolution == "" {
		resolution = helper.VideoResolutionTier(req.Size)
	}
	if resolution == "" {
		resolution = "720p"
	} else {
		resolution = normalizeSeedanceResolution(resolution)
	}

	duration := 5
	if meta.Duration != nil && int(*meta.Duration) > 0 {
		duration = int(*meta.Duration)
	} else if req.Duration > 0 {
		duration = req.Duration
	} else if seconds, err := strconv.Atoi(strings.TrimSpace(req.Seconds)); err == nil && seconds > 0 {
		duration = seconds
	}
	if duration > relaycommon.MaxTaskDurationSeconds {
		return seedanceRequestSettings{}, fmt.Errorf("duration %d exceeds max %d", duration, relaycommon.MaxTaskDurationSeconds)
	}

	return seedanceRequestSettings{
		Resolution: resolution,
		Duration:   duration,
		HasVideo:   hasVideoReference(req),
	}, nil
}

// ExtractUsageFacts 是计费事实接口的兼容包装（tiered 闸门按
// TaskUsageFactsProvider 判定，故两个接口都要实现）。
func (a *TaskAdaptor) ExtractUsageFacts(c *gin.Context, info *relaycommon.RelayInfo) map[string]any {
	facts, err := a.ExtractUsageFactsValidated(c, info)
	if err != nil {
		return map[string]any{}
	}
	return facts
}

// ExtractUsageFactsValidated 为 tiered_expr 任务计费提供提交时事实：
// tokens（按分辨率×时长估算，结算以上游实际 usage 覆盖）、resolution、
// video_input、unit_price（USD/1M）。非 Seedance 标准版返回空事实。
func (a *TaskAdaptor) ExtractUsageFactsValidated(c *gin.Context, info *relaycommon.RelayInfo) (map[string]any, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	modelName := req.Model
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	if !isSeedance25Model(modelName) && !isSeedance20Model(modelName) {
		return map[string]any{}, nil
	}

	settings, err := deriveSeedanceRequestSettings(&req)
	if err != nil {
		return nil, err
	}
	if err := validateSeedanceResolution(modelName, settings.Resolution); err != nil {
		return nil, err
	}
	unitPrice, ok := seedanceUnitPriceUSD(modelName, settings.Resolution, settings.HasVideo)
	if !ok {
		return nil, fmt.Errorf("%s: no token price for resolution %q", modelName, settings.Resolution)
	}
	tokens, ok := estimateSeedanceTokens(modelName, settings.Resolution, settings.Duration)
	if !ok {
		return nil, fmt.Errorf("%s: no token rate for resolution %q", modelName, settings.Resolution)
	}

	videoInput := "none"
	if settings.HasVideo {
		videoInput = "video"
	}
	return map[string]any{
		"tokens":      tokens,
		"resolution":  settings.Resolution,
		"video_input": videoInput,
		"unit_price":  unitPrice,
	}, nil
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

// 编译期锁定计费事实接口：tiered_expr 闸门按 TaskUsageFactsProvider 判定，
// 缺少任一接口会让 Seedance 表达式在提交时报“no usage expression or meter”。
var (
	_ channel.TaskUsageFactsProvider          = (*TaskAdaptor)(nil)
	_ channel.TaskValidatedUsageFactsProvider = (*TaskAdaptor)(nil)
)

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	// Accept only POST /v1/video/generations as "generate" action.
	// Ark 风格 content 数组请求由本渠道接受。
	return relaycommon.ValidateBasicTaskRequestWithArkContent(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 的优先级:模型配置了视频按秒表时由按秒表全权接管
// (table-wins,与 JS 插件任务适配器语义一致);未配表才回退本渠道内置的
// video_input 静态倍率表(按输出分辨率与是否含视频输入,相对基准价计费)。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if ratios := helper.VideoPriceRatiosFromTaskContext(c, info.OriginModelName); ratios != nil {
		return ratios
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	hasVideo := hasVideoReference(&req)
	// 与 convertToRequestPayload 的分辨率优先级一致:metadata > 顶层 > size 归一,
	// 否则 size-only 请求(convert 已把 1920x1080 归一为 1080p 发给上游)在静态表
	// 里选不到对应档位。
	resolution, _ := req.Metadata["resolution"].(string)
	if resolution == "" {
		resolution = req.Resolution
	}
	if resolution == "" {
		resolution = helper.VideoResolutionTier(req.Size)
	} else if tier := helper.VideoResolutionTier(resolution); tier != "" {
		resolution = tier
	}
	// 站点模型名（如 doubao-seedance-2-5-h）与上游名可能不同；静态倍率表按
	// 上游模型名维护，优先用映射后的名字查表。
	modelName := info.OriginModelName
	if info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	ratio, ok := GetVideoInputRatio(modelName, resolution, hasVideo)
	if !ok || ratio == 1.0 {
		return nil
	}
	return map[string]float64{"video_input": ratio}
}

// hasVideoReference 检查请求的顶层 content 数组或 metadata.content 是否包含
// video_url 条目，避免构建完整的上游 requestPayload。
func hasVideoReference(req *relaycommon.TaskSubmitReq) bool {
	for _, item := range req.Content {
		if item.Type == "video_url" || (item.Type == "" && item.VideoURL != nil) {
			return true
		}
	}
	if req.Metadata == nil {
		return false
	}
	contentRaw, ok := req.Metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// ParseResponse handles the upstream submit response and returns the parsed
// task result. It never writes to the client response — the controller presents
// ClientResponse after the durable task barrier and billing settlement.
func (a *TaskAdaptor) ParseResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*channel.TaskSubmitResponse, *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	// Parse Doubao response
	var dResp responsePayload
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		return nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}

	if dResp.ID == "" {
		return nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	// Ark 风格端点（/v1/videos/generations/tasks）返回 Ark 提交形态，
	// 其余路径保持 OpenAI 视频格式。
	var clientResponse any
	if relaycommon.IsArkVideoPath(c) {
		clientResponse = taskdto.NewArkVideoSubmit(info.PublicTaskID, info.OriginModelName, time.Now().Unix())
	} else {
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName
		clientResponse = ov
	}

	return &channel.TaskSubmitResponse{
		UpstreamTaskID: dResp.ID,
		TaskData:       responseBody,
		ClientResponse: clientResponse,
	}, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, task *model.Task, proxy string) (*http.Response, error) {
	if task == nil || task.GetUpstreamTaskID() == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	taskID := task.GetUpstreamTaskID()

	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// convertToRequestPayload 将网关统一请求转换为上游 Ark 内容协议：
//   - 客户端直传 content 数组（含 role 标注）时原样透传
//   - 否则 images 列表转换为 image_url 项，prompt 独占 text 项
//   - metadata 作为覆盖层后可整体替换 content / duration 等字段
//   - duration 优先级：metadata 覆盖 > 顶层 duration > OpenAI 风格 seconds
func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	if len(req.Content) > 0 {
		r.Content = append(r.Content, req.Content...)
	} else if req.HasImage() {
		// Add images if present
		for _, imgURL := range req.Images {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: lo.ToPtr(imgURL),
				},
			})
		}
	}

	metadata := req.Metadata
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	// 顶层 content 与 metadata.content 都是客户端直接给定的内容数组，
	// 其中的 text 项（即客户端提示词）都必须原样保留。
	_, contentFromClient := metadata["content"]
	contentFromClient = contentFromClient || len(req.Content) > 0

	settings, err := deriveSeedanceRequestSettings(req)
	if err != nil {
		return nil, err
	}
	if r.Duration == nil {
		r.Duration = lo.ToPtr(dto.IntValue(settings.Duration))
	}
	if r.Resolution == "" {
		// 保留既有的“无法解析就不下发”契约：上游按自己的缺省档出片，
		// 计费侧按 720p 估算、结算时按响应分辨率修正。
		if req.Resolution != "" {
			r.Resolution = normalizeSeedanceResolution(req.Resolution)
		} else if tier := helper.VideoResolutionTier(req.Size); tier != "" {
			r.Resolution = tier
		}
	} else {
		r.Resolution = normalizeSeedanceResolution(r.Resolution)
	}

	// ratio 与 Ark 规范一致默认 "adaptive"：上游在字段缺失时报
	// "ratio is required"，因此显式下发规范默认值。req.Ratio 为指针，
	// 以区分客户端缺省（nil）与显式空串；两者都落到 adaptive，空串
	// 不是合法 ratio。
	if r.Ratio == "" && req.Ratio != nil {
		r.Ratio = *req.Ratio
	}
	if r.Ratio == "" {
		r.Ratio = "adaptive"
	}

	if contentFromClient {
		// content 直传时其中的 text 项即提示词；仅在非空 text 缺失且顶层
		// prompt 非空时才补齐，避免改写客户端给定的提示词。空的 text 项
		// 不视为已携带提示词，直接用顶层 prompt 覆盖或追加。
		hasText := false
		emptyTextIdx := -1
		for i, c := range r.Content {
			if c.Type == "text" {
				if c.Text != nil && strings.TrimSpace(*c.Text) != "" {
					hasText = true
					break
				}
				if emptyTextIdx == -1 {
					emptyTextIdx = i
				}
			}
		}
		if !hasText && strings.TrimSpace(req.Prompt) != "" {
			if emptyTextIdx >= 0 {
				r.Content[emptyTextIdx].Text = lo.ToPtr(req.Prompt)
			} else {
				r.Content = append(r.Content, ContentItem{Type: "text", Text: lo.ToPtr(req.Prompt)})
			}
		}
	} else {
		r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
		r.Content = append(r.Content, ContentItem{
			Type: "text",
			Text: lo.ToPtr(req.Prompt),
		})
	}

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(task *model.Task, resp *http.Response, respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Doubao status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
		// 终态事实：用上游实际 tokens 覆盖提交时的估算值；分辨率/含视频输入
		// 取自提交快照，若响应带回了分辨率则按其修正 unit_price。
		tokens := resTask.Usage.CompletionTokens
		if tokens == 0 {
			tokens = resTask.Usage.TotalTokens
		}
		if tokens > 0 {
			facts := map[string]any{"tokens": float64(tokens)}
			if task != nil && task.PrivateData.BillingContext != nil && task.PrivateData.BillingContext.TieredSnapshot != nil {
				snapshotFacts := task.PrivateData.BillingContext.TieredSnapshot.UsageFacts
				resolution, _ := snapshotFacts["resolution"].(string)
				if resTask.Resolution != "" {
					if tier := helper.VideoResolutionTier(resTask.Resolution); tier != "" {
						resolution = tier
					}
				}
				videoInput, _ := snapshotFacts["video_input"].(string)
				modelName := task.Properties.UpstreamModelName
				if modelName == "" {
					modelName = task.Properties.OriginModelName
				}
				if resolution != "" {
					if unitPrice, ok := seedanceUnitPriceUSD(modelName, resolution, videoInput == "video"); ok {
						facts["unit_price"] = unitPrice
						facts["resolution"] = resolution
					}
				}
			}
			taskResult.UsageFacts = facts
		}
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
	case "expired", "cancelled":
		// 上游任务存在过期/取消终态，落到 default 会被当成
		// in_progress 永远轮询，统一映射为失败态。
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if resTask.Error.Message != "" {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = fmt.Sprintf("task %s", resTask.Status)
		}
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", dResp.Content.VideoURL)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if originTask.Status == model.TaskStatusFailure {
		// 失败/过期/取消等终态映射为失败后，转换结果必须携带 terminal error，
		// 否则客户端会把失败任务当成成功处理。优先用上游错误信息，
		// 为空时退回状态文案（与 ParseTaskResult 保持一致）。
		message := dResp.Error.Message
		if message == "" {
			message = fmt.Sprintf("task %s", dResp.Status)
		}
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: message,
			Code:    dResp.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}

// ConvertToArkVideo 将任务转换为 Ark 风格视频响应（/v1/videos/generations/tasks
// 查询端点的对外返回）。与 ConvertToOpenAIVideo 共用 originTask.Data 中缓存的
// 上游任务快照；终态失败时按原始上游状态区分 expired / failed。
func (a *TaskAdaptor) ConvertToArkVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao task data failed")
	}

	task := taskdto.ArkVideoTask{
		ID:        originTask.TaskID,
		Model:     originTask.Properties.OriginModelName,
		CreatedAt: originTask.CreatedAt,
		UpdatedAt: originTask.UpdatedAt,
	}

	switch originTask.Status {
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		task.Status = taskdto.ArkVideoStatusQueued
	case model.TaskStatusInProgress:
		task.Status = taskdto.ArkVideoStatusRunning
	case model.TaskStatusSuccess:
		task.Status = taskdto.ArkVideoStatusSucceeded
		if dResp.Content.VideoURL != "" {
			task.Content = &taskdto.ArkVideoContent{VideoURL: dResp.Content.VideoURL}
		}
		if dResp.Duration > 0 {
			task.Output = &taskdto.ArkVideoOutput{Duration: dResp.Duration}
		}
		if dResp.Usage.CompletionTokens > 0 {
			task.Usage = &taskdto.ArkVideoUsage{CompletionTokens: dResp.Usage.CompletionTokens}
		}
	case model.TaskStatusFailure:
		// expired/cancelled 等终态在 ParseTaskResult 阶段统一折叠为 FAILURE，
		// 这里依据缓存的原始上游状态区分 expired 与 failed。
		task.Status = taskdto.ArkVideoStatusFailed
		errorCode := taskdto.ArkVideoErrorFailed
		if dResp.Status == "expired" {
			task.Status = taskdto.ArkVideoStatusExpired
			errorCode = taskdto.ArkVideoErrorExpired
		}
		message := dResp.Error.Message
		if message == "" {
			message = fmt.Sprintf("task %s", dResp.Status)
		}
		task.Error = &taskdto.ArkVideoError{Code: errorCode, Message: message}
	}

	return common.Marshal(&task)
}
