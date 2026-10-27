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
package doubao

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	billingsetting "github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeedance25VideoPriceTable 锁定 Seedance 2.5 的官方 token 单价表：
// 480p/720p 不含视频输入 70 元/1M，1080p 77；含视频输入 42/42/46。
// 基准价（480p/720p 不含视频）为 1.0，其余档位返回相对基准的倍率。
func TestSeedance25VideoPriceTable(t *testing.T) {
	tests := []struct {
		resolution string
		hasVideo   bool
		want       float64
	}{
		{resolution: "480p", hasVideo: false, want: 1.0},
		{resolution: "720p", hasVideo: false, want: 1.0},
		{resolution: "1080p", hasVideo: false, want: 77.0 / 70.0},
		{resolution: "480p", hasVideo: true, want: 42.0 / 70.0},
		{resolution: "720p", hasVideo: true, want: 42.0 / 70.0},
		{resolution: "1080p", hasVideo: true, want: 46.0 / 70.0},
	}

	for _, tt := range tests {
		t.Run(tt.resolution+"-"+map[bool]string{true: "video", false: "no-video"}[tt.hasVideo], func(t *testing.T) {
			got, ok := GetVideoInputRatio("doubao-seedance-2-5-260628", tt.resolution, tt.hasVideo)
			require.True(t, ok)
			assert.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

// TestEstimateBillingResolvesMappedSeedance25Model 锁定映射后的模型名解析：
// 站点名 doubao-seedance-2-5-h 映射到上游 doubao-seedance-2-5-260628，静态
// video_input 倍率表必须按上游名命中，否则 1080p/含视频输入会按基准价计费。
func TestEstimateBillingResolvesMappedSeedance25Model(t *testing.T) {
	require.NoError(t, ratio_setting.UpdateVideoPriceByJSONString(`{}`))

	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:      "doubao-seedance-2-5-h",
		Resolution: "1080p",
		Duration:   5,
		Content: []relaycommon.TaskContentItem{
			{Type: "video_url", VideoURL: &relaycommon.TaskMediaURL{URL: lo.ToPtr("https://example.com/ref.mp4")}},
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", req)

	got := adaptor.EstimateBilling(c, &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-5-h",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "doubao-seedance-2-5-260628"},
	})
	assert.Equal(t, map[string]float64{"video_input": 46.0 / 70.0}, got)
}

// TestSeedanceUsageFacts 锁定 tiered_expr 的任务事实：提交时按分辨率/时长估算
// tokens，并按上游官方 token 单价给出 unit_price（USD/1M）；含参考视频时用
// 低价档位；2.5 不支持 4K，2.0 支持。
func TestSeedanceUsageFacts(t *testing.T) {
	adaptor := &TaskAdaptor{}
	videoRef := []relaycommon.TaskContentItem{
		{Type: "video_url", VideoURL: &relaycommon.TaskMediaURL{URL: lo.ToPtr("https://example.com/ref.mp4")}},
	}

	tests := []struct {
		name           string
		req            relaycommon.TaskSubmitReq
		upstream       string
		wantTokens     float64
		wantResolution string
		wantVideoInput string
		wantUnitPrice  float64
		wantErr        string
		wantEmpty      bool
	}{
		{
			name:           "2.5 480p text",
			req:            relaycommon.TaskSubmitReq{Model: "doubao-seedance-2-5-h", Prompt: "p", Resolution: "480p", Duration: 5},
			upstream:       "doubao-seedance-2-5-260628",
			wantTokens:     48437,
			wantResolution: "480p",
			wantVideoInput: "none",
			wantUnitPrice:  70.0 / 7.3,
		},
		{
			name:           "2.5 1080p with video input",
			req:            relaycommon.TaskSubmitReq{Model: "doubao-seedance-2-5-h", Prompt: "p", Resolution: "1080p", Duration: 5, Content: videoRef},
			upstream:       "doubao-seedance-2-5-260628",
			wantTokens:     245023,
			wantResolution: "1080p",
			wantVideoInput: "video",
			wantUnitPrice:  46.0 / 7.3,
		},
		{
			name:           "2.0 4k text",
			req:            relaycommon.TaskSubmitReq{Model: "doubao-seedance-2-0-h", Prompt: "p", Resolution: "4k", Duration: 5},
			upstream:       "doubao-seedance-2-0-260128",
			wantTokens:     972000,
			wantResolution: "4k",
			wantVideoInput: "none",
			wantUnitPrice:  26.0 / 7.3,
		},
		{
			name:     "2.5 rejects 4k",
			req:      relaycommon.TaskSubmitReq{Model: "doubao-seedance-2-5-h", Prompt: "p", Resolution: "4k", Duration: 5},
			upstream: "doubao-seedance-2-5-260628",
			wantErr:  "unsupported resolution",
		},
		{
			name:      "non seedance returns empty facts",
			req:       relaycommon.TaskSubmitReq{Model: "doubao-seedance-1-0-pro-250528", Prompt: "p", Resolution: "720p", Duration: 5},
			upstream:  "doubao-seedance-1-0-pro-250528",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", tt.req)
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.req.Model,
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: tt.upstream},
			}

			facts, err := adaptor.ExtractUsageFactsValidated(c, info)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantEmpty {
				assert.Empty(t, facts)
				return
			}

			assert.Equal(t, tt.wantTokens, facts["tokens"])
			assert.Equal(t, tt.wantResolution, facts["resolution"])
			assert.Equal(t, tt.wantVideoInput, facts["video_input"])
			assert.InDelta(t, tt.wantUnitPrice, facts["unit_price"], 1e-9)
		})
	}
}

// TestSeedanceUsageSchemaRegistered 锁定 init 注册的 u() 键集合与分辨率枚举：
// 2.5 不含 4k，2.0 含 4k；1.0 与 2.0-fast 不命中，避免把标准价套到 fast/mini。
func TestSeedanceUsageSchemaRegistered(t *testing.T) {
	schema25, ok := billingexpr.LookupTaskUsageSchema("doubao-seedance-2-5-h")
	require.True(t, ok)
	for _, key := range []string{"tokens", "resolution", "video_input", "unit_price"} {
		require.Contains(t, schema25, key)
	}
	assert.ElementsMatch(t, []string{"480p", "720p", "1080p"}, schema25["resolution"].Enum)

	schema20, ok := billingexpr.LookupTaskUsageSchema("doubao-seedance-2-0-260128")
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"480p", "720p", "1080p", "4k"}, schema20["resolution"].Enum)

	for _, model := range []string{"doubao-seedance-1-0-pro-250528", "doubao-seedance-2-0-fast-260128"} {
		_, ok = billingexpr.LookupTaskUsageSchema(model)
		require.False(t, ok, model)
	}
}

// TestParseTaskResultCarriesSeedanceTokenFacts 锁定终态事实：成功响应里的
// usage.completion_tokens/total_tokens 作为 tokens 事实并入 UsageFacts，
// 结算时用实际 tokens 重跑表达式；unit_price 按响应分辨率重新计算。
func TestParseTaskResultCarriesSeedanceTokenFacts(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		Properties: model.Properties{
			OriginModelName:   "doubao-seedance-2-5-h",
			UpstreamModelName: "doubao-seedance-2-5-260628",
		},
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				TieredSnapshot: &billingexpr.BillingSnapshot{
					UsageFacts: map[string]any{
						"resolution":  "480p",
						"video_input": "none",
						"unit_price":  70.0 / 7.3,
					},
				},
			},
		},
	}
	body := []byte(`{"id":"t1","status":"succeeded","resolution":"480p","usage":{"completion_tokens":48437,"total_tokens":48437}}`)

	info, err := adaptor.ParseTaskResult(task, nil, body)
	require.NoError(t, err)
	require.NotNil(t, info.UsageFacts)
	assert.Equal(t, float64(48437), info.UsageFacts["tokens"])
	assert.InDelta(t, 70.0/7.3, info.UsageFacts["unit_price"], 1e-9)
}

// TestSeedanceTokenBillingExpression 锁定站点要配置的任务表达式：
// tier("默认", u("tokens") * u("unit_price") / 1000000)。用 2.5 480p/5s
// 的实测 tokens 计算 quota，应与按秒表的历史结果一致（约 232231）。
func TestSeedanceTokenBillingExpression(t *testing.T) {
	const expr = `tier("默认", u("tokens") * u("unit_price") / 1000000)`

	for _, model := range []string{"doubao-seedance-2-5-h", "doubao-seedance-2-0-h"} {
		schema, ok := billingexpr.LookupTaskUsageSchema(model)
		require.True(t, ok, model)
		require.NoError(t, billingsetting.SmokeTestTaskExprWithFacts(expr, schema), model)
	}

	facts := map[string]any{"tokens": float64(48437), "unit_price": 70.0 / 7.3}
	cost, _, err := billingexpr.RunExprWithRequest(expr, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
	require.NoError(t, err)
	quota := common.QuotaRound(cost * common.QuotaPerUnit)
	assert.InDelta(t, 232231, quota, 5)
}
