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
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/relay/channel/minimax"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// contextKeyTTSFormat 保存请求成形后实际使用的音频格式，供响应写 Content-Type。
const contextKeyTTSFormat = "astraflow_tts_format"

// audioSpeechUsage 把上游的计费字符数写进 prompt_tokens：语音按字符计费，而
// 平台文本结算正好以 prompt_tokens 乘模型单价（tiered_expr 的 p 变量），字符数
// 因此能一对一映射到按字符定价，不经过音频 token 的时长折算。上游没给字符数
// （截断流、异常响应）时退回请求估算，避免整单免费。
func audioSpeechUsage(info *relaycommon.RelayInfo, usageCharacters int64) *dto.Usage {
	characters := minimax.ClampUsageCharacters(usageCharacters)
	if characters == 0 {
		characters = info.GetEstimatePromptTokens()
	}
	if characters == 0 {
		// CountToken 关闭时估算为 0；输入文本本身的字符数仍能兜住结算。
		if request, ok := info.Request.(*dto.AudioRequest); ok {
			characters = utf8.RuneCountInString(request.Input)
		}
	}
	return &dto.Usage{
		PromptTokens:        characters,
		CompletionTokens:    0,
		TotalTokens:         characters,
		PromptTokensDetails: dto.InputTokenDetails{TextTokens: characters},
	}
}

// ttsParseErrorStatus 区分上游拒绝（base_resp 非 0，客户端参数问题）与报文损坏
// （网关/上游故障）：前者 400，后者 500。
func ttsParseErrorStatus(err error) int {
	var upstreamErr *minimax.TTSUpstreamError
	if errors.As(err, &upstreamErr) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func handleAudioSpeechResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if info.IsStream {
		return handleAudioSpeechStreamResponse(c, resp, info)
	}

	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to read minimax response: %w", err),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}

	audio, audioURL, usageCharacters, parseErr := minimax.ParseTTSResponse(body)
	if parseErr != nil {
		return nil, types.NewErrorWithStatusCode(parseErr, types.ErrorCodeBadResponse, ttsParseErrorStatus(parseErr))
	}
	if audioURL != "" {
		c.Redirect(http.StatusFound, audioURL)
	} else {
		format := c.GetString(contextKeyTTSFormat)
		if format == "" {
			if request, ok := info.Request.(*dto.AudioRequest); ok && request.ResponseFormat != "" {
				format = request.ResponseFormat
			} else {
				format = "mp3"
			}
		}
		c.Data(http.StatusOK, minimax.ContentTypeForFormat(format), audio)
	}
	return audioSpeechUsage(info, usageCharacters), nil
}

func writeAudioSpeechDelta(c *gin.Context, audio []byte) error {
	return helper.ObjectData(c, map[string]any{
		"type":  "speech.audio.delta",
		"audio": base64.StdEncoding.EncodeToString(audio),
	})
}

func writeAudioSpeechDone(c *gin.Context, usage *dto.Usage) error {
	return helper.ObjectData(c, map[string]any{
		"type": "speech.audio.done",
		"usage": map[string]any{
			"input_tokens":  usage.PromptTokens,
			"output_tokens": usage.CompletionTokens,
			"total_tokens":  usage.TotalTokens,
		},
	})
}

// readUntilFirstNonEmptyLine 读出首个非空行（含它之前的所有空行），用于在进入
// SSE 扫描前识别上游是否根本没返回事件流。
func readUntilFirstNonEmptyLine(reader *bufio.Reader) (string, error) {
	var prefix strings.Builder
	for {
		line, err := reader.ReadString('\n')
		prefix.WriteString(line)
		if strings.TrimSpace(line) != "" {
			return prefix.String(), err
		}
		if err != nil {
			return prefix.String(), err
		}
	}
}

// handleAudioSpeechStreamResponse 把 MiniMax 的 hex 分片桥接成 OpenAI 的
// speech.audio.delta/speech.audio.done SSE 事件。上游收尾分片带 usage_characters，
// 是结算的唯一权威字符数；流被截断时保留请求估算。
func handleAudioSpeechStreamResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	reader := bufio.NewReader(resp.Body)
	prefix, _ := readUntilFirstNonEmptyLine(reader)

	// t2a_v2 对非法参数返回 HTTP 200 + 单条 JSON（没有 data: 前缀），不能当成
	// 空流静默成功：客户端拿不到音频，却会按估算字符数被扣费。
	if trimmed := strings.TrimSpace(prefix); !strings.HasPrefix(trimmed, "data:") {
		defer service.CloseResponseBodyGracefully(resp)
		rest, err := io.ReadAll(reader)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to read minimax response: %w", err),
				types.ErrorCodeReadResponseBodyFailed,
				http.StatusInternalServerError,
			)
		}
		body := append([]byte(prefix), rest...)
		audio, audioURL, usageCharacters, parseErr := minimax.ParseTTSResponse(body)
		if parseErr != nil {
			return nil, types.NewErrorWithStatusCode(parseErr, types.ErrorCodeBadResponse, ttsParseErrorStatus(parseErr))
		}
		usage := audioSpeechUsage(info, usageCharacters)
		if audioURL != "" {
			c.Redirect(http.StatusFound, audioURL)
			return usage, nil
		}
		// 客户端要的是 SSE；上游没按 stream 返回时把整段音频作为单个 delta 补发。
		helper.SetEventStreamHeaders(c)
		if err := writeAudioSpeechDelta(c, audio); err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeStreamDataHandler, http.StatusInternalServerError)
		}
		if err := writeAudioSpeechDone(c, usage); err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeStreamDataHandler, http.StatusInternalServerError)
		}
		return usage, nil
	}

	// 已读前缀拼回响应体，交给统一的流式扫描器（超时、心跳与断连处理）。
	resp.Body = struct {
		io.Reader
		io.Closer
	}{Reader: io.MultiReader(strings.NewReader(prefix), reader), Closer: resp.Body}

	usage := audioSpeechUsage(info, 0)
	doneSent := false
	failed := false

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		audio, status, usageCharacters, err := minimax.ParseTTSStreamChunk([]byte(data))
		if err != nil {
			failed = true
			sr.Error(err)
			return
		}
		if usageCharacters > 0 {
			usage = audioSpeechUsage(info, usageCharacters)
		}
		if len(audio) > 0 {
			if err := writeAudioSpeechDelta(c, audio); err != nil {
				failed = true
				sr.Error(err)
				return
			}
		}
		if status == minimax.TTSStreamStatusFinished {
			if err := writeAudioSpeechDone(c, usage); err != nil {
				failed = true
				sr.Error(err)
				return
			}
			doneSent = true
		}
	})

	if !doneSent && !failed {
		// 上游正常 EOF 但没发收尾分片时补一条 done；写不出去（客户端已断开）忽略。
		_ = writeAudioSpeechDone(c, usage)
	}
	return usage, nil
}
