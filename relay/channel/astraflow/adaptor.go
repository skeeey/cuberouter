package astraflow

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/security_setting"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("astraflow adaptor: relay info is nil")
	}
	if info.ChannelBaseUrl == "" {
		return "", errors.New("astraflow adaptor: channel base url is empty")
	}
	// 非任务中继会把渠道 API key 作为 Bearer 凭证发送到上游，拒绝明文上游
	// （https 或回环地址除外），避免凭证被明文传输（CWE-319）。
	if err := common.ValidateHTTPSChannelBaseURL(info.ChannelBaseUrl, security_setting.GetSecuritySetting().RequireHTTPSChannelBaseURL); err != nil {
		return "", fmt.Errorf("astraflow adaptor: %w", err)
	}
	requestPath := info.RequestURLPath
	if requestPath == "" {
		return info.ChannelBaseUrl, nil
	}
	// /v1/messages (RelayFormatClaude): 声明原生支持 messages 就透传客户端路径，
	// 否则请求体已被 ConvertClaudeRequest 转成 OpenAI 格式,必须打到 chat 端点。
	if info.RelayFormat == types.RelayFormatClaude &&
		info.RelayMode != constant.RelayModeResponses &&
		info.RelayMode != constant.RelayModeResponsesCompact {
		if useNativeProtocol(info, dto.ModelProtocolMessages) {
			return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestPath, info.ChannelType), nil
		}
		return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
	}
	// responses: 声明原生支持就透传,否则请求体已转成 chat,必须打到 chat 端点。
	if responsesDowngradedToChat(info) {
		return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestPath, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	// 非任务模式（chat/responses/embeddings/image）需要携带渠道鉴权头；
	// 若渠道已配置 Authorization Header Override 则跳过默认 Bearer，避免覆盖自定义鉴权。
	hasAuthOverride := false
	if len(info.HeadersOverride) > 0 {
		for k := range info.HeadersOverride {
			if strings.EqualFold(k, "Authorization") {
				hasAuthOverride = true
				break
			}
		}
	}
	if !hasAuthOverride {
		req.Set("Authorization", "Bearer "+info.ApiKey)
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// 声明原生支持 responses 的模型(含未配置时的现状基线)、以及改道来的会话
	// (见 responsesDowngradedToChat): 请求体原样转发。
	if !responsesDowngradedToChat(info) {
		return request, nil
	}
	// 其余模型: 降级为 chat 请求,响应侧由 OaiChatToResponses* 转回 Responses 形状。
	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, &request)
	if err != nil {
		return nil, err
	}
	aiRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	if info.SupportStreamOptions && info.IsStream {
		aiRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return aiRequest, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	// responses 的报文只有 Responses 处理器认得: chat 处理器会把响应体按 chat 解析,
	// 非流式拿不到 usage,流式会把 response.completed 收尾判成不完整流。
	// 声明原生 messages 的模型只在本次会话确实是 Anthropic Messages 时才交给
	// claude 处理器: 全局 ChatCompletionsToResponsesPolicy 会把 Claude 格式请求
	// 改道 Responses 协议(relay/claude_handler.go),那类响应不是 Anthropic 形状。
	switch {
	case info.RelayFormat == types.RelayFormatClaude &&
		useNativeProtocol(info, dto.ModelProtocolMessages) &&
		info.RelayMode != constant.RelayModeResponses &&
		info.RelayMode != constant.RelayModeResponsesCompact:
		return (&claude.Adaptor{}).DoResponse(c, resp, info)
	case info.RelayMode == constant.RelayModeResponses:
		if responsesDowngradedToChat(info) {
			// 上游收到的是 chat 响应,转成 Responses 形状给客户端。
			if info.IsStream {
				return openai.OaiChatToResponsesStreamHandler(c, info, resp)
			}
			return openai.OaiChatToResponsesHandler(c, info, resp)
		}
		if info.IsStream {
			return openai.OaiResponsesStreamHandler(c, info, resp)
		}
		return openai.OaiResponsesHandler(c, info, resp)
	default:
		if info.IsStream {
			return openai.OaiStreamHandler(c, info, resp)
		}
		return openai.OpenaiHandler(c, info, resp)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	// 声明原生支持 messages 的模型: 请求体原样转发,响应由 claude 处理器解析。
	if useNativeProtocol(info, dto.ModelProtocolMessages) {
		return request, nil
	}
	// 上游只说 OpenAI 协议: 把 Anthropic 请求体转成 OpenAI chat 请求，
	// 由 GetRequestURL 指向 /v1/chat/completions；响应侧由 openai handler
	// 按 RelayFormatClaude 转回 Anthropic 格式（含流式逐块转换）。
	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, request)
	if err != nil {
		return nil, err
	}
	aiRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	if info.SupportStreamOptions && info.IsStream {
		aiRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return a.ConvertOpenAIRequest(c, info, aiRequest)
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// upstreamModelID 返回发往上游的模型名。
func upstreamModelID(info *relaycommon.RelayInfo) string {
	if info != nil && info.UpstreamModelName != "" {
		return info.UpstreamModelName
	}
	if info == nil {
		return ""
	}
	return info.OriginModelName
}

// useNativeProtocol 报告本次请求能否直连上游的该协议。
// 必须区分"未命中声明"与"命中但不含该协议": 两者现状基线不同——messages 的现状
// 是被转成 chat(基线 false),responses 的现状是原样直连(基线 true)。未配置时
// 即按基线走,保证零回归;命中时以声明为准。
func useNativeProtocol(info *relaycommon.RelayInfo, protocol string) bool {
	if info == nil || info.ChannelMeta == nil {
		return protocol == dto.ModelProtocolResponses
	}
	protocols, declared := info.ChannelOtherSettings.ResolveModelProtocols(upstreamModelID(info))
	if !declared {
		return protocol == dto.ModelProtocolResponses
	}
	return slices.Contains(protocols, protocol)
}

// responsesDowngradedToChat 报告本次会话的 Responses 请求是否需要降级为 chat 发出
// (请求体转 chat、打 chat 端点、响应由 OaiChatToResponses* 转回)。
// 只有客户端确实以 Responses 协议发起的会话才降级: 全局
// ChatCompletionsToResponsesPolicy 会把 chat/messages 客户端改道到 Responses 协议
// (relay/chat_completions_via_responses.go),那类会话的 RelayMode 同样是 Responses,
// 但上游报文本来就是 Responses,响应侧由 host 的 OaiResponsesToChat* 固定按
// Responses 解析且不经过本适配器的 DoResponse——改了请求形状就会与响应解析错配。
// 请求体透传时同样不降级: host 不会调用 ConvertOpenAIResponsesRequest
// (relay/responses_handler.go),上游收到的是客户端原始的 Responses 报文,
// 只能打 Responses 端点。
func responsesDowngradedToChat(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.RelayFormat == types.RelayFormatOpenAIResponses &&
		info.RelayMode == constant.RelayModeResponses &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		!model_setting.GetGlobalSettings().PassThroughRequestEnabled &&
		!useNativeProtocol(info, dto.ModelProtocolResponses)
}

// Ensure compile-time interface check.
var _ channel.Adaptor = (*Adaptor)(nil)
