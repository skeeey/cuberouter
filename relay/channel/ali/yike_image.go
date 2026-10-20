/*
Copyright (C) 2023-2026 QuantumNous
Copyright (C) 2026 CubeRouter

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package ali

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 阿里云万镜一刻（Yike）与 DashScope/Bailian 的图片 API 是两套协议：
//   - 业务参数全部放在 RPC query 中，POST 根路径 /，body 为空；
//   - ApiKey 认证使用 Authorization: Bearer <key>，同时必须带 Timestamp；
//   - 提交返回 JobId，必须轮询 GetImageGenerationJob 才能拿到图片。
//
// 该协议仍挂在 Ali（type 17）渠道下，按渠道 base_url 主机名分流，避免为
// 单一上游新增渠道类型；万镜一刻的主机形如 yike.<region>.aliyuncs.com。
const (
	yikeImageAPIVersion    = "2026-07-07"
	yikeSubmitImageAction  = "SubmitImageGenerationJob"
	yikeGetImageAction     = "GetImageGenerationJob"
	yikeImageMaxN          = 4
	yikeImageDefaultScene  = "general"
	yikeImageDefaultRes    = "1K"
	yikeImageDefaultAspect = "1:1"
)

var (
	// 变量而非常量：测试需要压缩轮询间隔。
	yikeInitialPollDelay = 2 * time.Second
	yikePollInterval     = 5 * time.Second
	yikePollMaxAttempts  = 60
	yikePollTimeout      = 10 * time.Minute
	yikePollClient       = &http.Client{Timeout: 60 * time.Second}
)

func isYikeBaseURL(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.HasPrefix(host, "yike.") && strings.HasSuffix(host, ".aliyuncs.com")
}

// yikeRPCURL 只保留 scheme 与 host：万镜一刻的所有 Action 都挂在根路径下。
func yikeRPCURL(baseURL string, query url.Values) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid Yike base URL: %w", err)
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid Yike base URL %q", baseURL)
	}
	return scheme + "://" + u.Host + "/?" + query.Encode(), nil
}

type yikeImageOptions struct {
	Resolution   string `json:"resolution"`
	AspectRatio  string `json:"aspect_ratio"`
	N            *int   `json:"n"`
	PromptExtend *bool  `json:"prompt_extend"`
}

func convertYikeImageRequest(info *relaycommon.RelayInfo, request dto.ImageRequest) (url.Values, error) {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	var options yikeImageOptions
	if raw, ok := request.Extra["parameters"]; ok {
		if err := common.Unmarshal(raw, &options); err != nil {
			return nil, fmt.Errorf("invalid parameters field: %w", err)
		}
	}

	n, err := yikeImageCount(request, options)
	if err != nil {
		return nil, err
	}

	resolution := strings.ToUpper(strings.TrimSpace(options.Resolution))
	if resolution == "" {
		resolution = yikeImageDefaultRes
	}
	switch resolution {
	case "1K", "2K", "4K":
	default:
		return nil, fmt.Errorf("resolution must be one of 1K, 2K, 4K")
	}

	aspectRatio := strings.TrimSpace(options.AspectRatio)
	if aspectRatio == "" {
		if mapped, ok := yikeAspectRatioFromSize(request.Size); ok {
			aspectRatio = mapped
		} else {
			aspectRatio = yikeImageDefaultAspect
		}
	}
	switch aspectRatio {
	case "16:9", "9:16", "4:3", "3:4", "1:1", "21:9":
	default:
		return nil, fmt.Errorf("aspect_ratio must be one of 16:9, 9:16, 4:3, 3:4, 1:1, 21:9")
	}

	model := strings.TrimSpace(request.Model)
	if info != nil && info.ChannelMeta != nil {
		if upstream := strings.TrimSpace(info.UpstreamModelName); upstream != "" {
			model = upstream
		}
	}
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	inputJSON, err := common.Marshal(struct {
		Prompt string `json:"Prompt"`
	}{Prompt: prompt})
	if err != nil {
		return nil, fmt.Errorf("marshal Yike image input failed: %w", err)
	}

	query := url.Values{}
	query.Set("Action", yikeSubmitImageAction)
	query.Set("Version", yikeImageAPIVersion)
	query.Set("Timestamp", time.Now().UTC().Format(time.RFC3339))
	query.Set("Format", "json")
	query.Set("JobType", "text_to_image")
	query.Set("Model", model)
	query.Set("Scene", yikeImageDefaultScene)
	query.Set("Input", string(inputJSON))
	query.Set("Resolution", resolution)
	query.Set("AspectRatio", aspectRatio)
	query.Set("N", strconv.Itoa(n))
	if token := yikeImageClientToken(info); token != "" {
		// 同一个网关请求的多次重试复用同一个 ClientToken，避免上游重复扣费。
		query.Set("ClientToken", token)
	}
	if options.PromptExtend != nil {
		jobParameters, err := common.Marshal(map[string]any{"PromptExtend": *options.PromptExtend})
		if err != nil {
			return nil, fmt.Errorf("marshal Yike job parameters failed: %w", err)
		}
		query.Set("JobParameters", string(jobParameters))
	}
	return query, nil
}

func yikeImageCount(request dto.ImageRequest, options yikeImageOptions) (int, error) {
	n := 1
	if request.N != nil && *request.N > 0 {
		// 先比较再转换：超大的 uint 直接转 int 可能回绕成负数。
		if *request.N > uint(yikeImageMaxN) {
			return 0, fmt.Errorf("n must be between 1 and %d", yikeImageMaxN)
		}
		n = int(*request.N)
	}
	if options.N != nil {
		n = *options.N
	}
	if n < 1 || n > yikeImageMaxN {
		return 0, fmt.Errorf("n must be between 1 and %d", yikeImageMaxN)
	}
	return n, nil
}

// yikeAspectRatioFromSize 把 OpenAI 常见的 size 写法映射到万镜一刻的宽高比。
// 未识别的 size 返回 false，由调用方落到默认 1:1。
func yikeAspectRatioFromSize(size string) (string, bool) {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(size), " ", "")) {
	case "256x256", "512x512", "1024x1024", "2048x2048":
		return "1:1", true
	case "1024x576", "1280x720", "1536x1024", "1792x1024", "1920x1080", "2048x1152":
		return "16:9", true
	case "576x1024", "720x1280", "1024x1536", "1024x1792", "1080x1920", "1152x2048":
		return "9:16", true
	case "1024x768", "1536x1152", "1600x1200", "2048x1536":
		return "4:3", true
	case "768x1024", "1152x1536", "1200x1600", "1536x2048":
		return "3:4", true
	case "2100x900", "2560x1080", "3440x1440":
		return "21:9", true
	}
	return "", false
}

func yikeImageClientToken(info *relaycommon.RelayInfo) string {
	if info == nil || strings.TrimSpace(info.RequestId) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("yike-image:" + info.RequestId))
	return hex.EncodeToString(sum[:16])
}

type yikeSubmitResponse struct {
	RequestId string `json:"RequestId"`
	JobId     string `json:"JobId"`
	Code      string `json:"Code"`
	Message   string `json:"Message"`
}

type yikeImageJob struct {
	JobId        string `json:"JobId"`
	Status       string `json:"Status"`
	Output       string `json:"Output"`
	ErrorMessage string `json:"ErrorMessage"`
	Resolution   string `json:"Resolution"`
	N            string `json:"N"`
}

type yikeJobResponse struct {
	RequestId          string        `json:"RequestId"`
	ImageGenerationJob *yikeImageJob `json:"ImageGenerationJob"`
	Code               string        `json:"Code"`
	Message            string        `json:"Message"`
}

type yikeJobOutput struct {
	Medias []struct {
		MediaId   string `json:"MediaId"`
		OutputUrl string `json:"OutputUrl"`
	} `json:"Medias"`
}

type yikeHTTPError struct {
	StatusCode int
	Body       string
}

func (e *yikeHTTPError) Error() string {
	return fmt.Sprintf("Yike returned HTTP %d: %s", e.StatusCode, e.Body)
}

func yikeImageHandler(a *Adaptor, c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}
	service.CloseResponseBodyGracefully(resp)

	var submit yikeSubmitResponse
	if err := common.Unmarshal(responseBody, &submit); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}
	if submit.Code != "" || submit.Message != "" {
		return types.NewError(fmt.Errorf("yike submit error %s: %s", submit.Code, submit.Message), types.ErrorCodeBadResponse), nil
	}
	if strings.TrimSpace(submit.JobId) == "" {
		return types.NewError(fmt.Errorf("yike response did not contain JobId"), types.ErrorCodeBadResponse), nil
	}

	job, _, err := waitYikeImageJob(c, a.yikeBaseURL, info.ApiKey, submit.JobId)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponse), nil
	}
	if job.Status == "Failed" {
		message := strings.TrimSpace(job.ErrorMessage)
		if message == "" {
			message = "image generation failed"
		}
		return types.WithOpenAIError(types.OpenAIError{
			Message: message,
			Type:    "ali_error",
			Code:    "yike_image_failed",
		}, http.StatusBadGateway), nil
	}

	var output yikeJobOutput
	if err := common.Unmarshal([]byte(job.Output), &output); err != nil {
		return types.NewError(fmt.Errorf("invalid Yike image output: %w", err), types.ErrorCodeBadResponse), nil
	}

	imageResponse := dto.ImageResponse{Created: info.StartTime.Unix()}
	for _, media := range output.Medias {
		if strings.TrimSpace(media.OutputUrl) == "" {
			continue
		}
		imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: media.OutputUrl})
	}
	if len(imageResponse.Data) == 0 {
		return types.NewError(fmt.Errorf("Yike finished task did not contain any image"), types.ErrorCodeBadResponse), nil
	}

	// 按上游实际返回张数结算：请求 N 与上游实际产出不一致时以产出为准。
	info.PriceData.AddOtherRatio("n", float64(len(imageResponse.Data)))

	jsonResponse, err := common.Marshal(imageResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return nil, &dto.Usage{}
}

func waitYikeImageJob(c *gin.Context, baseURL, apiKey, jobId string) (*yikeImageJob, []byte, error) {
	time.Sleep(yikeInitialPollDelay)

	var lastBody []byte
	var lastErr error
	deadline := time.Now().Add(yikePollTimeout)
	for attempt := 0; attempt < yikePollMaxAttempts; attempt++ {
		if time.Now().After(deadline) {
			break
		}
		job, body, err := fetchYikeImageJob(baseURL, apiKey, jobId)
		lastBody = body
		if err != nil {
			lastErr = err
			var httpErr *yikeHTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 && httpErr.StatusCode != http.StatusTooManyRequests {
				return nil, lastBody, err
			}
			logger.LogWarn(c, fmt.Sprintf("yike image task %s poll failed: %v", jobId, err))
		} else {
			lastErr = nil
			switch job.Status {
			case "Finished", "Failed":
				return job, lastBody, nil
			case "Created", "Queuing", "Executing", "":
				// 继续等待
			default:
				logger.LogWarn(c, fmt.Sprintf("yike image task %s returned unknown status %q", jobId, job.Status))
			}
		}
		if attempt+1 < yikePollMaxAttempts {
			time.Sleep(yikePollInterval)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("yike image task %s timed out", jobId)
	}
	return nil, lastBody, lastErr
}

func fetchYikeImageJob(baseURL, apiKey, jobId string) (*yikeImageJob, []byte, error) {
	query := url.Values{}
	query.Set("Action", yikeGetImageAction)
	query.Set("Version", yikeImageAPIVersion)
	query.Set("Timestamp", time.Now().UTC().Format(time.RFC3339))
	query.Set("Format", "json")
	query.Set("JobId", jobId)

	endpoint, err := yikeRPCURL(baseURL, query)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := yikePollClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	service.CloseResponseBodyGracefully(resp)
	if readErr != nil {
		return nil, body, readErr
	}
	if resp.StatusCode != http.StatusOK {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return nil, body, &yikeHTTPError{StatusCode: resp.StatusCode, Body: snippet}
	}

	var result yikeJobResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, body, fmt.Errorf("unmarshal Yike image job failed: %w", err)
	}
	if result.Code != "" || result.Message != "" {
		return nil, body, fmt.Errorf("yike poll error %s: %s", result.Code, result.Message)
	}
	if result.ImageGenerationJob == nil {
		return nil, body, fmt.Errorf("Yike response did not contain ImageGenerationJob")
	}
	return result.ImageGenerationJob, body, nil
}
