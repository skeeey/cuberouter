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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsYikeBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    bool
	}{
		{name: "shanghai", baseURL: "https://yike.cn-shanghai.aliyuncs.com/", want: true},
		{name: "singapore", baseURL: "https://yike.ap-southeast-1.aliyuncs.com", want: true},
		{name: "maas workspace", baseURL: "https://ws-fvd7g7b5rmahd1d8.cn-beijing.maas.aliyuncs.com", want: false},
		{name: "dashscope", baseURL: "https://dashscope.aliyuncs.com", want: false},
		{name: "lookalike host", baseURL: "https://yike.cn-shanghai.aliyuncs.com.evil.example.com", want: false},
		{name: "path contains yike", baseURL: "https://example.com/yike", want: false},
		{name: "invalid", baseURL: "not-a-url", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isYikeBaseURL(tt.baseURL))
		})
	}
}

func TestConvertYikeImageRequest(t *testing.T) {
	info := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RequestId: "req-123",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "wan2.7-image",
			},
		}
	}
	uintPtr := func(v uint) *uint { return &v }
	extra := func(raw string) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		require.NoError(t, common.UnmarshalJsonStr(raw, &m))
		return m
	}

	tests := []struct {
		name       string
		request    dto.ImageRequest
		wantN      string
		wantRes    string
		wantAspect string
		wantErr    string
	}{
		{
			name:       "defaults",
			request:    dto.ImageRequest{Prompt: "一只猫"},
			wantN:      "1",
			wantRes:    "1K",
			wantAspect: "1:1",
		},
		{
			name:       "request n",
			request:    dto.ImageRequest{Prompt: "一只猫", N: uintPtr(3)},
			wantN:      "3",
			wantRes:    "1K",
			wantAspect: "1:1",
		},
		{
			name:       "size maps to aspect ratio",
			request:    dto.ImageRequest{Prompt: "一只猫", Size: "1792x1024"},
			wantN:      "1",
			wantRes:    "1K",
			wantAspect: "16:9",
		},
		{
			name: "parameters override",
			request: dto.ImageRequest{
				Prompt: "一只猫",
				N:      uintPtr(4),
				Extra:  extra(`{"parameters":{"n":2,"resolution":"2K","aspect_ratio":"9:16"}}`),
			},
			wantN:      "2",
			wantRes:    "2K",
			wantAspect: "9:16",
		},
		{
			name:    "invalid n from request",
			request: dto.ImageRequest{Prompt: "一只猫", N: uintPtr(5)},
			wantErr: "n must be between 1 and 4",
		},
		{
			name:    "invalid n from parameters",
			request: dto.ImageRequest{Prompt: "一只猫", Extra: extra(`{"parameters":{"n":0}}`)},
			wantErr: "n must be between 1 and 4",
		},
		{
			name:    "invalid resolution",
			request: dto.ImageRequest{Prompt: "一只猫", Extra: extra(`{"parameters":{"resolution":"8K"}}`)},
			wantErr: "resolution must be one of",
		},
		{
			name:    "invalid aspect ratio",
			request: dto.ImageRequest{Prompt: "一只猫", Extra: extra(`{"parameters":{"aspect_ratio":"2:1"}}`)},
			wantErr: "aspect_ratio must be one of",
		},
		{
			name:    "empty prompt",
			request: dto.ImageRequest{Prompt: "   "},
			wantErr: "prompt is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := convertYikeImageRequest(info(), tt.request)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, yikeSubmitImageAction, query.Get("Action"))
			assert.Equal(t, yikeImageAPIVersion, query.Get("Version"))
			assert.Equal(t, "json", query.Get("Format"))
			assert.Equal(t, "text_to_image", query.Get("JobType"))
			assert.Equal(t, "wan2.7-image", query.Get("Model"))
			assert.Equal(t, "general", query.Get("Scene"))
			assert.Equal(t, tt.wantN, query.Get("N"))
			assert.Equal(t, tt.wantRes, query.Get("Resolution"))
			assert.Equal(t, tt.wantAspect, query.Get("AspectRatio"))
			assert.Contains(t, query.Get("Input"), tt.request.Prompt)
			assert.Len(t, query.Get("ClientToken"), 32)
		})
	}
}

func TestConvertYikeImageRequestJobParameters(t *testing.T) {
	request := dto.ImageRequest{
		Prompt: "一只猫",
		Extra: map[string]json.RawMessage{
			"parameters": json.RawMessage(`{"prompt_extend":true}`),
		},
	}
	query, err := convertYikeImageRequest(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "wan2.7-image"},
	}, request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"PromptExtend":true}`, query.Get("JobParameters"))
}

func TestConvertYikeImageRequestModelFallsBackToRequestModel(t *testing.T) {
	query, err := convertYikeImageRequest(&relaycommon.RelayInfo{}, dto.ImageRequest{
		Model:  "wan2.7-image-a",
		Prompt: "一只猫",
	})
	require.NoError(t, err)
	assert.Equal(t, "wan2.7-image-a", query.Get("Model"))
}

func TestYikeClientTokenIsStablePerRequest(t *testing.T) {
	first := yikeImageClientToken(&relaycommon.RelayInfo{RequestId: "req-1"})
	second := yikeImageClientToken(&relaycommon.RelayInfo{RequestId: "req-1"})
	other := yikeImageClientToken(&relaycommon.RelayInfo{RequestId: "req-2"})
	assert.Equal(t, first, second)
	assert.NotEqual(t, first, other)
	assert.Len(t, first, 32)
	assert.Empty(t, yikeImageClientToken(nil))
}

func TestAdaptorYikeImageRequestURLAndEmptyBody(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: constant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://yike.cn-shanghai.aliyuncs.com/",
			UpstreamModelName: "wan2.7-image",
		},
	}
	adaptor.Init(info)

	body, err := adaptor.ConvertImageRequest(nil, info, dto.ImageRequest{Prompt: "一只猫"})
	require.NoError(t, err)
	buffer, ok := body.(*bytes.Buffer)
	require.True(t, ok)
	assert.Zero(t, buffer.Len())

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	parsed, err := url.Parse(requestURL)
	require.NoError(t, err)
	assert.Equal(t, "yike.cn-shanghai.aliyuncs.com", parsed.Host)
	assert.Equal(t, yikeSubmitImageAction, parsed.Query().Get("Action"))
	assert.Equal(t, "wan2.7-image", parsed.Query().Get("Model"))
}

func TestAdaptorYikeRejectsImageEdits(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: constant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://yike.cn-shanghai.aliyuncs.com/",
		},
	}
	adaptor.Init(info)
	_, err := adaptor.ConvertImageRequest(nil, info, dto.ImageRequest{Prompt: "一只猫"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestYikeImageHandlerFinished(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("Action"); got != yikeGetImageAction {
			t.Errorf("unexpected action %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("unexpected authorization %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = io.WriteString(w, `{"RequestId":"r1","ImageGenerationJob":{"JobId":"job-1","Status":"Executing"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"RequestId":"r2","ImageGenerationJob":{"JobId":"job-1","Status":"Finished","Output":"{\"Medias\":[{\"MediaId\":\"m1\",\"OutputUrl\":\"https://cdn.example.com/a.png\"},{\"MediaId\":\"m2\",\"OutputUrl\":\"https://cdn.example.com/b.png\"}]}"}}`)
	}))
	defer server.Close()
	restoreYikePollTiming(t)

	adaptor := &Adaptor{isYike: true, yikeBaseURL: server.URL}
	info := newYikeTestRelayInfo()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	apiErr, usage := yikeImageHandler(adaptor, c, newYikeSubmitResponse(`{"RequestId":"sub","JobId":"job-1"}`), info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, float64(2), info.PriceData.OtherRatios()["n"])

	var response dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
	assert.Equal(t, "https://cdn.example.com/a.png", response.Data[0].Url)
	assert.Equal(t, "https://cdn.example.com/b.png", response.Data[1].Url)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestYikeImageHandlerFailures(t *testing.T) {
	tests := []struct {
		name       string
		submitBody string
		status     int
		jobBody    string
		wantErr    string
	}{
		{
			name:       "submit error",
			submitBody: `{"RequestId":"sub","Code":"InvalidParameter.Model","Message":"Model is not available"}`,
			wantErr:    "Model is not available",
		},
		{
			name:       "missing job id",
			submitBody: `{"RequestId":"sub"}`,
			wantErr:    "did not contain JobId",
		},
		{
			name:       "job failed",
			submitBody: `{"RequestId":"sub","JobId":"job-1"}`,
			jobBody:    `{"RequestId":"r","ImageGenerationJob":{"JobId":"job-1","Status":"Failed","ErrorMessage":"prompt rejected"}}`,
			wantErr:    "prompt rejected",
		},
		{
			name:       "invalid output",
			submitBody: `{"RequestId":"sub","JobId":"job-1"}`,
			jobBody:    `{"RequestId":"r","ImageGenerationJob":{"JobId":"job-1","Status":"Finished","Output":"not-json"}}`,
			wantErr:    "invalid Yike image output",
		},
		{
			name:       "empty medias",
			submitBody: `{"RequestId":"sub","JobId":"job-1"}`,
			jobBody:    `{"RequestId":"r","ImageGenerationJob":{"JobId":"job-1","Status":"Finished","Output":"{\"Medias\":[]}"}}`,
			wantErr:    "did not contain any image",
		},
		{
			name:       "permanent poll error",
			submitBody: `{"RequestId":"sub","JobId":"job-1"}`,
			status:     http.StatusUnauthorized,
			wantErr:    "HTTP 401",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.status
			if status == 0 {
				status = http.StatusOK
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, tt.jobBody)
			}))
			defer server.Close()
			restoreYikePollTiming(t)

			adaptor := &Adaptor{isYike: true, yikeBaseURL: server.URL}
			info := newYikeTestRelayInfo()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			apiErr, _ := yikeImageHandler(adaptor, c, newYikeSubmitResponse(tt.submitBody), info)
			require.NotNil(t, apiErr)
			assert.Contains(t, apiErr.Error(), tt.wantErr)
		})
	}
}

func TestYikeImageHandlerTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"RequestId":"r","ImageGenerationJob":{"JobId":"job-1","Status":"Executing"}}`)
	}))
	defer server.Close()
	restoreYikePollTiming(t)
	previousAttempts := yikePollMaxAttempts
	yikePollMaxAttempts = 2
	t.Cleanup(func() { yikePollMaxAttempts = previousAttempts })

	adaptor := &Adaptor{isYike: true, yikeBaseURL: server.URL}
	info := newYikeTestRelayInfo()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	apiErr, _ := yikeImageHandler(adaptor, c, newYikeSubmitResponse(`{"RequestId":"sub","JobId":"job-1"}`), info)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "timed out")
}

func newYikeTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		StartTime: time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "test-key",
		},
	}
}

func newYikeSubmitResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func restoreYikePollTiming(t *testing.T) {
	t.Helper()
	previousInitialDelay := yikeInitialPollDelay
	previousInterval := yikePollInterval
	previousAttempts := yikePollMaxAttempts
	yikeInitialPollDelay = 0
	yikePollInterval = time.Millisecond
	yikePollMaxAttempts = 5
	t.Cleanup(func() {
		yikeInitialPollDelay = previousInitialDelay
		yikePollInterval = previousInterval
		yikePollMaxAttempts = previousAttempts
	})
}
