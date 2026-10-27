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
package openai

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSTTResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func newSTTContext() *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
	return c
}

func TestOpenaiSTTHandlerUsageSettlement(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		body               string
		estimatePrompt     int
		wantPromptTokens   int
		wantCompleteTokens int
	}{
		{
			// MOSS 按音频时长计费：回显的 prompt_tokens 与秒数不成比例，
			// 结算改按 input_audio_seconds 折算 whisper-1 的 1000 token/分钟口径。
			name:               "duration usage settled by audio seconds",
			body:               `{"usage":{"type":"duration","seconds":4,"input_audio_seconds":3.76,"prompt_tokens":133,"completion_tokens":40,"total_tokens":173},"text":"你好"}`,
			estimatePrompt:     999,
			wantPromptTokens:   67, // ceil(3.76)/60*1000 = 66.67 → 67
			wantCompleteTokens: 0,
		},
		{
			name:               "duration usage falls back to seconds field",
			body:               `{"usage":{"type":"duration","seconds":10,"prompt_tokens":214,"total_tokens":215}}`,
			estimatePrompt:     999,
			wantPromptTokens:   167, // ceil(10)/60*1000 = 166.67 → 167
			wantCompleteTokens: 0,
		},
		{
			name:               "duration usage without seconds keeps upstream tokens",
			body:               `{"usage":{"type":"duration","seconds":0,"input_audio_seconds":0,"prompt_tokens":124,"total_tokens":125}}`,
			estimatePrompt:     999,
			wantPromptTokens:   124,
			wantCompleteTokens: 0,
		},
		{
			name:               "token usage keeps upstream tokens",
			body:               `{"usage":{"type":"tokens","input_tokens":10,"output_tokens":5,"total_tokens":15}}`,
			estimatePrompt:     999,
			wantPromptTokens:   10,
			wantCompleteTokens: 5,
		},
		{
			name:               "missing usage falls back to request estimate",
			body:               `{"text":"你好"}`,
			estimatePrompt:     42,
			wantPromptTokens:   42,
			wantCompleteTokens: 0,
		},
		{
			// 时长来自上游，可能被伪造成天文数字：饱和到单请求上限而不是回绕成负数。
			name:               "absurd duration saturates",
			body:               `{"usage":{"type":"duration","seconds":1000000000000,"input_audio_seconds":1000000000000,"total_tokens":1}}`,
			estimatePrompt:     999,
			wantPromptTokens:   math.MaxInt32,
			wantCompleteTokens: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{}
			info.SetEstimatePromptTokens(tt.estimatePrompt)

			err, usage := OpenaiSTTHandler(newSTTContext(), newSTTResponse(tt.body), info, "json")

			require.Nil(t, err)
			require.NotNil(t, usage)
			assert.Equal(t, tt.wantPromptTokens, usage.PromptTokens)
			assert.Equal(t, tt.wantCompleteTokens, usage.CompletionTokens)
		})
	}
}
