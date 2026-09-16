package astraflow

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetNativeProtocolDowngradeMemoForTest() {
	nativeProtocolDowngradeMemo.Range(func(key, _ any) bool {
		nativeProtocolDowngradeMemo.Delete(key)
		return true
	})
}

// TestConvertOpenAIRequestPassthrough 锁定 OpenAI 直传契约：消息请求必须原样
// 转发给上游，nil 请求必须报错而不是 panic。
func TestConvertOpenAIRequestPassthrough(t *testing.T) {
	adaptor := &Adaptor{}
	request := &dto.GeneralOpenAIRequest{Model: "deepseek-v3"}

	out, err := adaptor.ConvertOpenAIRequest(nil, nil, request)
	require.NoError(t, err)
	got, ok := out.(*dto.GeneralOpenAIRequest)
	require.True(t, ok, "expected *dto.GeneralOpenAIRequest, got %T", out)
	assert.Equal(t, request, got)

	_, err = adaptor.ConvertOpenAIRequest(nil, nil, nil)
	require.Error(t, err)
}

// TestConvertOpenAIResponsesRequestPassthrough 锁定 /v1/responses 直传契约：
// Responses 请求必须原样转发给上游，而不是返回 not implemented。
func TestConvertOpenAIResponsesRequestPassthrough(t *testing.T) {
	adaptor := &Adaptor{}
	request := dto.OpenAIResponsesRequest{Model: "deepseek-v3"}

	out, err := adaptor.ConvertOpenAIResponsesRequest(nil, nil, request)
	require.NoError(t, err)
	got, ok := out.(dto.OpenAIResponsesRequest)
	require.True(t, ok, "expected dto.OpenAIResponsesRequest, got %T", out)
	assert.Equal(t, request, got)
}

// TestConvertEmbeddingRequestPassthrough 锁定 /v1/embeddings 直传契约：
// Embedding 请求必须原样转发给上游，而不是返回 not implemented。
func TestConvertEmbeddingRequestPassthrough(t *testing.T) {
	adaptor := &Adaptor{}
	request := dto.EmbeddingRequest{Model: "text-embedding-3-large", Input: []string{"hello"}}

	out, err := adaptor.ConvertEmbeddingRequest(nil, nil, request)
	require.NoError(t, err)
	got, ok := out.(dto.EmbeddingRequest)
	require.True(t, ok, "expected dto.EmbeddingRequest, got %T", out)
	assert.Equal(t, request, got)
}

// TestConvertImageRequestPassthrough 锁定 /v1/images/generations 直传契约：
// Image 请求必须原样转发给上游，而不是返回 not implemented。
func TestConvertImageRequestPassthrough(t *testing.T) {
	adaptor := &Adaptor{}
	request := dto.ImageRequest{Model: "gpt-image-1", Prompt: "a cat on a boat"}

	out, err := adaptor.ConvertImageRequest(nil, nil, request)
	require.NoError(t, err)
	got, ok := out.(dto.ImageRequest)
	require.True(t, ok, "expected dto.ImageRequest, got %T", out)
	assert.Equal(t, request, got)
}

// TestGetRequestURLForwardsRequestPath 锁定 URL 直传契约：AstraFlow 各模式下
// 上游 URL 直接沿用客户端请求路径（/v1/chat/completions、/v1/responses、
// /v1/embeddings、/v1/images/generations），保证多模态请求到达正确端点。
func TestGetRequestURLForwardsRequestPath(t *testing.T) {
	adaptor := &Adaptor{}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "chat completions", path: "/v1/chat/completions", want: "https://api.modelverse.cn/v1/chat/completions"},
		{name: "responses", path: "/v1/responses", want: "https://api.modelverse.cn/v1/responses"},
		{name: "embeddings", path: "/v1/embeddings", want: "https://api.modelverse.cn/v1/embeddings"},
		{name: "image generations", path: "/v1/images/generations", want: "https://api.modelverse.cn/v1/images/generations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl: "https://api.modelverse.cn",
					ChannelType:    constant.ChannelTypeAstraFlow,
				},
				RequestURLPath: tt.path,
			}

			url, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, tt.want, url)
		})
	}
}

// TestAstraflowDowngradesRejectedNativeProtocol 锁定运行时兜底: 配置声明原生
// 支持 messages,但上游用 "not implemented" 明确拒绝时,该组合在进程内被记住,
// 后续请求不再直连,改为降级转换;且探测读取的错误体必须被还原,不影响调用方解析。
func TestAstraflowDowngradesRejectedNativeProtocol(t *testing.T) {
	resetNativeProtocolDowngradeMemoForTest()
	t.Cleanup(resetNativeProtocolDowngradeMemoForTest)

	gin.SetMode(gin.TestMode)

	const upstreamErrorBody = `{"error":{"message":"not implemented","type":"upstream_error"}}`
	const requestBody = `{"model":"glm-5.3","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(upstreamErrorBody))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(requestBody))

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         7,
			ChannelBaseUrl:    server.URL,
			ChannelType:       constant.ChannelTypeAstraFlow,
			ApiKey:            "sk-test",
			UpstreamModelName: "glm-5.3",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ModelProtocols: map[string][]string{"glm-*": {dto.ModelProtocolChat, dto.ModelProtocolMessages}},
			},
		},
		RequestURLPath:  "/v1/messages",
		RelayFormat:     types.RelayFormatClaude,
		RelayMode:       relayconstant.RelayModeChatCompletions,
		OriginModelName: "glm-5.3",
	}
	adaptor := &Adaptor{}

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, server.URL+"/v1/messages", url, "declared native messages must be used before the first failure")

	respAny, err := adaptor.DoRequest(context, info, bytes.NewBufferString(requestBody))
	require.NoError(t, err)
	resp, ok := respAny.(*http.Response)
	require.True(t, ok, "expected *http.Response, got %T", respAny)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, upstreamErrorBody, string(body), "peeking must restore the response body for the caller")

	url, err = adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/chat/completions", url, "after a rejection the combination must downgrade to chat")

	converted, err := adaptor.ConvertClaudeRequest(context, info, &dto.ClaudeRequest{Model: "glm-5.3"})
	require.NoError(t, err)
	_, isClaudeRequest := converted.(*dto.ClaudeRequest)
	assert.False(t, isClaudeRequest, "downgraded requests must be converted to chat")
}
