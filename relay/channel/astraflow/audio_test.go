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
package astraflow

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAudioSpeechInfo 构造一次 /v1/audio/speech 的 AstraFlow 会话。
func newAudioSpeechInfo(streamFormat string) *relaycommon.RelayInfo {
	request := &dto.AudioRequest{
		Model:          "speech-2.8-hd",
		Input:          "你好",
		Voice:          "male-qn-qingse",
		ResponseFormat: "mp3",
		StreamFormat:   streamFormat,
	}
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://api.modelverse.cn",
			ChannelType:       constant.ChannelTypeAstraFlow,
			UpstreamModelName: "speech-2.8-hd",
		},
		RelayMode:       relayconstant.RelayModeAudioSpeech,
		RelayFormat:     types.RelayFormatOpenAIAudio,
		Request:         request,
		IsStream:        streamFormat == "sse",
		OriginModelName: "speech-2.8-hd",
	}
}

// newAudioSpeechContext 构造带真实请求的 gin 上下文（流式扫描会读 c.Request）。
func newAudioSpeechContext(recorder *httptest.ResponseRecorder) *gin.Context {
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	return c
}

func audioPayload(t *testing.T, reader io.Reader) map[string]any {
	t.Helper()
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	return payload
}

// TestConvertAudioRequestBuildsMiniMaxPayload 锁定 OpenAI speech 到 MiniMax
// t2a_v2 的请求成形：模型用渠道映射后的上游名，音频编码走 audio_setting，
// 响应固定 hex（上游只接受 hex/url，hex 才能由网关解码成音频字节）。
func TestConvertAudioRequestBuildsMiniMaxPayload(t *testing.T) {
	adaptor := &Adaptor{}
	info := newAudioSpeechInfo("")
	c := newAudioSpeechContext(httptest.NewRecorder())
	request := *info.Request.(*dto.AudioRequest)
	speed := 1.2
	request.Speed = &speed

	reader, err := adaptor.ConvertAudioRequest(c, info, request)
	require.NoError(t, err)
	payload := audioPayload(t, reader)

	assert.Equal(t, "speech-2.8-hd", payload["model"])
	assert.Equal(t, "你好", payload["text"])
	assert.Equal(t, "hex", payload["output_format"])
	voice, ok := payload["voice_setting"].(map[string]any)
	require.True(t, ok, "voice_setting must be an object")
	assert.Equal(t, "male-qn-qingse", voice["voice_id"])
	assert.Equal(t, 1.2, voice["speed"])
	audioSetting, ok := payload["audio_setting"].(map[string]any)
	require.True(t, ok, "audio_setting must be an object")
	assert.Equal(t, "mp3", audioSetting["format"])
	_, hasStream := payload["stream"]
	assert.False(t, hasStream, "non-SSE requests must not ask upstream for a stream")
}

// TestConvertAudioRequestMarksSSEStream 锁定流式契约：客户端用 sse 时向上游
// 打开 stream，并要求上游不再在收尾分片重复聚合音频（否则 SSE 桥接会把整段
// 音频再发一遍）。
func TestConvertAudioRequestMarksSSEStream(t *testing.T) {
	adaptor := &Adaptor{}
	info := newAudioSpeechInfo("sse")
	c := newAudioSpeechContext(httptest.NewRecorder())

	reader, err := adaptor.ConvertAudioRequest(c, info, *info.Request.(*dto.AudioRequest))
	require.NoError(t, err)
	payload := audioPayload(t, reader)

	assert.Equal(t, true, payload["stream"])
	streamOptions, ok := payload["stream_options"].(map[string]any)
	require.True(t, ok, "stream_options must be an object")
	assert.Equal(t, true, streamOptions["exclude_aggregated_audio"])
}

// TestConvertAudioRequestKeepsProtocolFieldsOverMetadata 锁定计费/安全字段的
// 优先级：metadata 只能扩展厂商参数（音色、情绪等），不得改写本次请求的模型、
// 文本、stream 与响应编码——否则模型可按便宜价计费、文本可绕过敏感词检查。
func TestConvertAudioRequestKeepsProtocolFieldsOverMetadata(t *testing.T) {
	adaptor := &Adaptor{}
	info := newAudioSpeechInfo("sse")
	c := newAudioSpeechContext(httptest.NewRecorder())
	request := *info.Request.(*dto.AudioRequest)
	request.Metadata = json.RawMessage(`{"model":"billed-cheap","text":"different","output_format":"url","stream":false,"voice_setting":{"emotion":"happy","pitch":5}}`)

	reader, err := adaptor.ConvertAudioRequest(c, info, request)
	require.NoError(t, err)
	payload := audioPayload(t, reader)

	assert.Equal(t, "speech-2.8-hd", payload["model"])
	assert.Equal(t, "你好", payload["text"])
	assert.Equal(t, "hex", payload["output_format"])
	assert.Equal(t, true, payload["stream"])
	voice, ok := payload["voice_setting"].(map[string]any)
	require.True(t, ok, "voice_setting must be an object")
	assert.Equal(t, "happy", voice["emotion"])
	assert.Equal(t, float64(5), voice["pitch"])
}

func TestConvertAudioRequestRejectsNonSpeechMode(t *testing.T) {
	adaptor := &Adaptor{}
	info := newAudioSpeechInfo("")
	info.RelayMode = relayconstant.RelayModeAudioTranscription

	_, err := adaptor.ConvertAudioRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, *info.Request.(*dto.AudioRequest))
	require.Error(t, err)
}

func TestGetRequestURLForAudioSpeechUsesMiniMaxT2AEndpoint(t *testing.T) {
	adaptor := &Adaptor{}
	info := newAudioSpeechInfo("")

	got, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.modelverse.cn/v1/t2a_v2", got)
}

// TestAudioSpeechNonStreamResponse 锁定非流式响应：hex 音频解码为二进制、
// Content-Type 跟随请求格式，usage 以上游 usage_characters 作为计费字符数
// （prompt_tokens 承载字符数，供 tiered_expr 的 p 变量计价）。
func TestAudioSpeechNonStreamResponse(t *testing.T) {
	adaptor := &Adaptor{}
	audio := []byte{0x49, 0x44, 0x33, 0x04}
	body := `{"data":{"audio":"` + hex.EncodeToString(audio) + `"},"extra_info":{"usage_characters":4},"base_resp":{"status_code":0}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	recorder := httptest.NewRecorder()
	c := newAudioSpeechContext(recorder)
	info := newAudioSpeechInfo("")

	usageAny, apiErr := adaptor.DoResponse(c, resp, info)
	require.Nil(t, apiErr)
	usage, ok := usageAny.(*dto.Usage)
	require.True(t, ok, "usage must be *dto.Usage, got %T", usageAny)

	assert.Equal(t, 4, usage.PromptTokens)
	assert.Equal(t, 0, usage.CompletionTokens)
	assert.Equal(t, 4, usage.TotalTokens)
	assert.Equal(t, audio, recorder.Body.Bytes())
	assert.Equal(t, "audio/mpeg", recorder.Header().Get("Content-Type"))
}

func TestAudioSpeechNonStreamResponseSurfacesUpstreamError(t *testing.T) {
	adaptor := &Adaptor{}
	body := `{"base_resp":{"status_code":2013,"status_msg":"invalid params, param 'output_format' only supports 'hex' and 'url'"}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	c := newAudioSpeechContext(httptest.NewRecorder())
	info := newAudioSpeechInfo("")

	_, apiErr := adaptor.DoResponse(c, resp, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "2013")
}

// TestAudioSpeechNonStreamResponseMalformedBodyIsServerError 区分上游故障与参数
// 错误：非 JSON 报文是网关/上游故障，不能回报 400 让客户端误以为请求有问题。
func TestAudioSpeechNonStreamResponseMalformedBodyIsServerError(t *testing.T) {
	adaptor := &Adaptor{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("not json")),
	}
	c := newAudioSpeechContext(httptest.NewRecorder())
	info := newAudioSpeechInfo("")

	_, apiErr := adaptor.DoResponse(c, resp, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

// TestAudioSpeechUsageRejectsOutOfRangeCharacters 锁定计费安全边界：上游字符数
// 是 int64，超上限必须饱和到 int32（不能回绕成负数），负数交给请求估算兜底。
func TestAudioSpeechUsageRejectsOutOfRangeCharacters(t *testing.T) {
	tests := []struct {
		name       string
		characters string
		want       int
	}{
		{name: "max int64 saturates", characters: "9223372036854775807", want: math.MaxInt32},
		{name: "negative falls back to estimate", characters: "-5", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adaptor := &Adaptor{}
			body := `{"data":{"audio":"49"},"extra_info":{"usage_characters":` + tt.characters + `},"base_resp":{"status_code":0}}`
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}
			c := newAudioSpeechContext(httptest.NewRecorder())
			info := newAudioSpeechInfo("")

			usageAny, apiErr := adaptor.DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			usage, ok := usageAny.(*dto.Usage)
			require.True(t, ok, "usage must be *dto.Usage, got %T", usageAny)
			assert.Equal(t, tt.want, usage.PromptTokens)
			assert.Equal(t, tt.want, usage.TotalTokens)
		})
	}
}

// withStreamingTimeout 给 StreamScannerHandler 一个正数超时（生产由 InitEnv 设置）。
func withStreamingTimeout(t *testing.T) {
	t.Helper()
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
}

func parseSSEEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event map[string]any
		require.NoError(t, common.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event))
		events = append(events, event)
	}
	return events
}

// TestAudioSpeechStreamResponseBridgesHexChunks 锁定流式桥接：MiniMax 的 hex
// 分片要转成 OpenAI speech.audio.delta（base64），收尾的 usage_characters 要
// 转成 speech.audio.done 并成为计费字符数。
func TestAudioSpeechStreamResponseBridgesHexChunks(t *testing.T) {
	withStreamingTimeout(t)
	adaptor := &Adaptor{}
	chunk1 := []byte{0x49, 0x44, 0x33}
	chunk2 := []byte{0x04, 0x00, 0xff}
	sse := "data: {\"data\":{\"audio\":\"" + hex.EncodeToString(chunk1) + "\",\"status\":1},\"base_resp\":{\"status_code\":0}}\n\n" +
		"data: {\"data\":{\"audio\":\"" + hex.EncodeToString(chunk2) + "\",\"status\":1},\"base_resp\":{\"status_code\":0}}\n\n" +
		"data: {\"data\":{\"audio\":\"\",\"status\":2},\"extra_info\":{\"usage_characters\":4},\"base_resp\":{\"status_code\":0,\"status_msg\":\"success\"}}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(sse)),
	}
	recorder := httptest.NewRecorder()
	c := newAudioSpeechContext(recorder)
	info := newAudioSpeechInfo("sse")

	usageAny, apiErr := adaptor.DoResponse(c, resp, info)
	require.Nil(t, apiErr)
	usage, ok := usageAny.(*dto.Usage)
	require.True(t, ok, "usage must be *dto.Usage, got %T", usageAny)
	assert.Equal(t, 4, usage.PromptTokens)

	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	events := parseSSEEvents(t, recorder.Body.String())
	require.Len(t, events, 3)

	assert.Equal(t, "speech.audio.delta", events[0]["type"])
	firstAudio, err := base64.StdEncoding.DecodeString(events[0]["audio"].(string))
	require.NoError(t, err)
	assert.Equal(t, chunk1, firstAudio)

	assert.Equal(t, "speech.audio.delta", events[1]["type"])
	secondAudio, err := base64.StdEncoding.DecodeString(events[1]["audio"].(string))
	require.NoError(t, err)
	assert.Equal(t, chunk2, secondAudio)

	assert.Equal(t, "speech.audio.done", events[2]["type"])
	doneUsage, ok := events[2]["usage"].(map[string]any)
	require.True(t, ok, "done event must carry usage")
	assert.Equal(t, float64(4), doneUsage["total_tokens"])
}

// TestAudioSpeechStreamResponseSurfacesJSONError 锁定上游流式请求失败时的行为：
// t2a_v2 对非法参数返回 HTTP 200 + 单条 JSON（没有 data: 前缀），不能被当成空
// 流静默成功，否则客户端拿不到音频却仍按估算扣费。
func TestAudioSpeechStreamResponseSurfacesJSONError(t *testing.T) {
	withStreamingTimeout(t)
	adaptor := &Adaptor{}
	body := `{"base_resp":{"status_code":2013,"status_msg":"invalid params, empty field"}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	recorder := httptest.NewRecorder()
	c := newAudioSpeechContext(recorder)
	info := newAudioSpeechInfo("sse")

	_, apiErr := adaptor.DoResponse(c, resp, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "2013")
	assert.Empty(t, recorder.Body.String(), "must not emit SSE events for an upstream error")
}

// TestAudioSpeechStreamResponseWrapsNonStreamUpstreamSuccess 锁定 SSE 会话的兜底：
// 上游若没按 stream 返回而是给了单条 JSON（data: 前缀缺失但 base_resp 成功），
// 也要按客户端声明的 stream_format=sse 输出 delta/done，不能把裸音频写给 SSE 客户端。
func TestAudioSpeechStreamResponseWrapsNonStreamUpstreamSuccess(t *testing.T) {
	withStreamingTimeout(t)
	adaptor := &Adaptor{}
	audio := []byte{0x49, 0x44, 0x33}
	body := `{"data":{"audio":"` + hex.EncodeToString(audio) + `"},"extra_info":{"usage_characters":4},"base_resp":{"status_code":0}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	recorder := httptest.NewRecorder()
	c := newAudioSpeechContext(recorder)
	info := newAudioSpeechInfo("sse")

	usageAny, apiErr := adaptor.DoResponse(c, resp, info)
	require.Nil(t, apiErr)
	usage, ok := usageAny.(*dto.Usage)
	require.True(t, ok, "usage must be *dto.Usage, got %T", usageAny)
	assert.Equal(t, 4, usage.PromptTokens)

	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	events := parseSSEEvents(t, recorder.Body.String())
	require.Len(t, events, 2)
	assert.Equal(t, "speech.audio.delta", events[0]["type"])
	gotAudio, err := base64.StdEncoding.DecodeString(events[0]["audio"].(string))
	require.NoError(t, err)
	assert.Equal(t, audio, gotAudio)
	assert.Equal(t, "speech.audio.done", events[1]["type"])
}

// TestAudioSpeechStreamResponseFallsBackToEstimate 锁定截断流的计费兜底：
// 客户端中途断开、上游没发收尾分片时，按请求估算的字符数结算，不能整单免费；
// CountToken 关闭（估算为 0）时退回输入文本的字符数。
func TestAudioSpeechStreamResponseFallsBackToEstimate(t *testing.T) {
	withStreamingTimeout(t)
	sse := "data: {\"data\":{\"audio\":\"" + hex.EncodeToString([]byte{0x49}) + "\",\"status\":1},\"base_resp\":{\"status_code\":0}}\n\n"

	tests := []struct {
		name     string
		estimate int
		want     int
	}{
		{name: "uses request estimate", estimate: 7, want: 7},
		{name: "falls back to input rune count", estimate: 0, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adaptor := &Adaptor{}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(sse)),
			}
			c := newAudioSpeechContext(httptest.NewRecorder())
			info := newAudioSpeechInfo("sse")
			info.SetEstimatePromptTokens(tt.estimate)

			usageAny, apiErr := adaptor.DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			usage, ok := usageAny.(*dto.Usage)
			require.True(t, ok, "usage must be *dto.Usage, got %T", usageAny)
			assert.Equal(t, tt.want, usage.PromptTokens)
			assert.Equal(t, tt.want, usage.TotalTokens)
		})
	}
}
