package minimax

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type MiniMaxTTSRequest struct {
	Model             string             `json:"model"`
	Text              string             `json:"text"`
	Stream            bool               `json:"stream,omitempty"`
	StreamOptions     *StreamOptions     `json:"stream_options,omitempty"`
	VoiceSetting      VoiceSetting       `json:"voice_setting"`
	PronunciationDict *PronunciationDict `json:"pronunciation_dict,omitempty"`
	AudioSetting      *AudioSetting      `json:"audio_setting,omitempty"`
	TimbreWeights     []TimbreWeight     `json:"timbre_weights,omitempty"`
	LanguageBoost     string             `json:"language_boost,omitempty"`
	VoiceModify       *VoiceModify       `json:"voice_modify,omitempty"`
	SubtitleEnable    bool               `json:"subtitle_enable,omitempty"`
	OutputFormat      string             `json:"output_format,omitempty"`
	AigcWatermark     bool               `json:"aigc_watermark,omitempty"`
}

type StreamOptions struct {
	ExcludeAggregatedAudio bool `json:"exclude_aggregated_audio,omitempty"`
}

type VoiceSetting struct {
	VoiceID           string  `json:"voice_id"`
	Speed             float64 `json:"speed,omitempty"`
	Vol               float64 `json:"vol,omitempty"`
	Pitch             int     `json:"pitch,omitempty"`
	Emotion           string  `json:"emotion,omitempty"`
	TextNormalization bool    `json:"text_normalization,omitempty"`
	LatexRead         bool    `json:"latex_read,omitempty"`
}

type PronunciationDict struct {
	Tone []string `json:"tone,omitempty"`
}

type AudioSetting struct {
	SampleRate int    `json:"sample_rate,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Format     string `json:"format,omitempty"`
	Channel    int    `json:"channel,omitempty"`
	ForceCbr   bool   `json:"force_cbr,omitempty"`
}

type TimbreWeight struct {
	VoiceID string `json:"voice_id"`
	Weight  int    `json:"weight"`
}

type VoiceModify struct {
	Pitch        int    `json:"pitch,omitempty"`
	Intensity    int    `json:"intensity,omitempty"`
	Timbre       int    `json:"timbre,omitempty"`
	SoundEffects string `json:"sound_effects,omitempty"`
}

type MiniMaxTTSResponse struct {
	Data      MiniMaxTTSData   `json:"data"`
	ExtraInfo MiniMaxExtraInfo `json:"extra_info"`
	TraceID   string           `json:"trace_id"`
	BaseResp  MiniMaxBaseResp  `json:"base_resp"`
}

type MiniMaxTTSData struct {
	Audio  string `json:"audio"`
	Status int    `json:"status"`
}

type MiniMaxExtraInfo struct {
	UsageCharacters int64 `json:"usage_characters"`
}

type MiniMaxBaseResp struct {
	StatusCode int64  `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// TTSStreamStatusFinished 是 t2a_v2 流式收尾分片的 status 值。
const TTSStreamStatusFinished = 2

// TTSUpstreamError 表示 t2a_v2 以 base_resp 拒绝了请求（通常是参数或音色非法），
// 调用方据此把它映射成客户端错误而不是网关/上游故障。
type TTSUpstreamError struct {
	Code    int64
	Message string
}

func (e *TTSUpstreamError) Error() string {
	return fmt.Sprintf("minimax TTS error: %d - %s", e.Code, e.Message)
}

// BuildTTSRequest 把 OpenAI speech 请求成形为 MiniMax t2a_v2 报文。model 传
// 渠道映射后的上游模型名。模型、文本、流式开关与响应编码（output_format 只接受
// hex/url）属于协议与计费字段，metadata 只能扩展厂商参数（音色、情绪、音频格式
// 等），不能改写它们——否则可按便宜模型计费，或让文本绕过网关的敏感词检查。
func BuildTTSRequest(request dto.AudioRequest, model string, stream bool) (MiniMaxTTSRequest, error) {
	format := request.ResponseFormat
	if format == "" {
		format = "mp3"
	}
	minimaxRequest := MiniMaxTTSRequest{
		Model: model,
		Text:  request.Input,
		VoiceSetting: VoiceSetting{
			VoiceID: request.Voice,
			Speed:   lo.FromPtrOr(request.Speed, 0.0),
		},
		AudioSetting: &AudioSetting{
			Format: format,
		},
		OutputFormat: "hex",
	}

	if len(request.Metadata) > 0 {
		if err := common.Unmarshal(request.Metadata, &minimaxRequest); err != nil {
			return MiniMaxTTSRequest{}, fmt.Errorf("error unmarshalling metadata to minimax request: %w", err)
		}
	}

	minimaxRequest.Model = model
	minimaxRequest.Text = request.Input
	minimaxRequest.OutputFormat = "hex"
	if stream {
		minimaxRequest.Stream = true
		minimaxRequest.StreamOptions = &StreamOptions{ExcludeAggregatedAudio: true}
	} else {
		minimaxRequest.Stream = false
		minimaxRequest.StreamOptions = nil
	}
	if minimaxRequest.AudioSetting == nil {
		minimaxRequest.AudioSetting = &AudioSetting{Format: format}
	} else if minimaxRequest.AudioSetting.Format == "" {
		minimaxRequest.AudioSetting.Format = format
	}

	return minimaxRequest, nil
}

// ParseTTSResponse 解析非流式 t2a_v2 响应。audio 为解码后的音频字节；
// output_format=url 时 audioURL 非空，由调用方决定是重定向还是取回。
func ParseTTSResponse(body []byte) (audio []byte, audioURL string, usageCharacters int64, err error) {
	var response MiniMaxTTSResponse
	if err = common.Unmarshal(body, &response); err != nil {
		return nil, "", 0, fmt.Errorf("failed to unmarshal minimax TTS response: %w", err)
	}
	if response.BaseResp.StatusCode != 0 {
		return nil, "", 0, &TTSUpstreamError{Code: response.BaseResp.StatusCode, Message: response.BaseResp.StatusMsg}
	}
	if response.Data.Audio == "" {
		return nil, "", 0, errors.New("no audio data in minimax TTS response")
	}
	if strings.HasPrefix(response.Data.Audio, "http") {
		return nil, response.Data.Audio, response.ExtraInfo.UsageCharacters, nil
	}
	audio, err = hex.DecodeString(response.Data.Audio)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to decode hex audio data: %w", err)
	}
	return audio, "", response.ExtraInfo.UsageCharacters, nil
}

// ParseTTSStreamChunk 解析 t2a_v2 的单个流式分片（payload 已去掉 "data: " 前缀）。
// 收尾分片 status 为 TTSStreamStatusFinished，并携带 usage_characters。
func ParseTTSStreamChunk(payload []byte) (audio []byte, status int, usageCharacters int64, err error) {
	var event MiniMaxTTSResponse
	if err = common.Unmarshal(payload, &event); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to unmarshal minimax TTS stream chunk: %w", err)
	}
	if event.BaseResp.StatusCode != 0 {
		return nil, 0, 0, &TTSUpstreamError{Code: event.BaseResp.StatusCode, Message: event.BaseResp.StatusMsg}
	}
	if event.Data.Audio == "" {
		return nil, event.Data.Status, event.ExtraInfo.UsageCharacters, nil
	}
	audio, err = hex.DecodeString(event.Data.Audio)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to decode hex audio chunk: %w", err)
	}
	return audio, event.Data.Status, event.ExtraInfo.UsageCharacters, nil
}

// ContentTypeForFormat 把请求的音频格式映射为响应 Content-Type。
func ContentTypeForFormat(format string) string {
	contentTypeMap := map[string]string{
		"mp3":  "audio/mpeg",
		"wav":  "audio/wav",
		"flac": "audio/flac",
		"aac":  "audio/aac",
		"opus": "audio/ogg",
		"pcm":  "audio/pcm",
	}
	if ct, ok := contentTypeMap[format]; ok {
		return ct
	}
	return "audio/mpeg" // default to mp3
}

// ClampUsageCharacters 把上游返回的计费字符数收敛到 int32 边界，避免上游异常值
// 溢出成负数或绕过单请求配额上限。
func ClampUsageCharacters(value int64) int {
	if value <= 0 {
		return 0
	}
	if value > math.MaxInt32 {
		common.SysError(fmt.Sprintf("minimax usage_characters out of range: %d", value))
		return math.MaxInt32
	}
	return int(value)
}

func handleTTSResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to read minimax response: %w", readErr),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	audio, audioURL, usageCharacters, parseErr := ParseTTSResponse(body)
	if parseErr != nil {
		status := http.StatusInternalServerError
		var upstreamErr *TTSUpstreamError
		if errors.As(parseErr, &upstreamErr) {
			status = http.StatusBadRequest
		}
		return nil, types.NewErrorWithStatusCode(parseErr, types.ErrorCodeBadResponse, status)
	}

	format := c.GetString("response_format")
	if format == "" {
		format = "mp3"
	}
	if audioURL != "" {
		c.Redirect(http.StatusFound, audioURL)
	} else {
		c.Data(http.StatusOK, ContentTypeForFormat(format), audio)
	}

	usage = &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      ClampUsageCharacters(usageCharacters),
	}
	return usage, nil
}

func handleChatCompletionResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to read minimax response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	// Set response headers
	for key, values := range resp.Header {
		if !service.ShouldCopyUpstreamHeader(c, key, values) {
			continue
		}
		for _, value := range values {
			c.Header(key, value)
		}
	}

	c.Data(resp.StatusCode, "application/json", body)
	return nil, nil
}
