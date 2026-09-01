package vidu

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// beijing returns a fixed time instant expressed in Asia/Shanghai, so tests
// stay deterministic regardless of the machine's local timezone.
func beijing(layout, value string) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	t, err := time.ParseInLocation(layout, value, loc)
	if err != nil {
		panic(err)
	}
	return t
}

// seedVideoPrice 注入与硬编码时代一致的管理员视频价格表(12 格全部对应),
// 使配置驱动的推导结果与既有期望逐格一致。
func seedVideoPrice(t *testing.T) {
	t.Helper()
	err := ratio_setting.UpdateVideoPriceByJSONString(`{
  "viduq3-pro": {"rows": [
    {"resolution":"1080p","normal_price":0.75,"off_peak_price":0.375},
    {"resolution":"720p","normal_price":0.625,"off_peak_price":0.3125},
    {"resolution":"540p","normal_price":0.28125,"off_peak_price":0.15625}]},
  "viduq3-turbo": {"rows": [
    {"resolution":"1080p","normal_price":0.40625,"off_peak_price":0.21875},
    {"resolution":"720p","normal_price":0.375,"off_peak_price":0.1875},
    {"resolution":"540p","normal_price":0.21875,"off_peak_price":0.125}]}
}`)
	require.NoError(t, err)
}

func TestIsViduOffPeakHour(t *testing.T) {
	// 错峰窗口:北京时间 [22:00, 次日 08:00)
	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"peak_0800_boundary", beijing("2006-01-02 15:04:05", "2026-09-01 08:00:00"), false},
		{"offpeak_0759", beijing("2006-01-02 15:04:05", "2026-09-01 07:59:59"), true},
		{"peak_midday", beijing("2006-01-02 15:04:05", "2026-09-01 12:00:00"), false},
		{"peak_2159", beijing("2006-01-02 15:04:05", "2026-09-01 21:59:59"), false},
		{"offpeak_2200_boundary", beijing("2006-01-02 15:04:05", "2026-09-01 22:00:00"), true},
		{"offpeak_2359", beijing("2006-01-02 15:04:05", "2026-09-01 23:59:59"), true},
		{"offpeak_midnight", beijing("2006-01-02 15:04:05", "2026-09-01 00:00:00"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isViduOffPeakHour(tt.now))
		})
	}
}

func TestEstimateBillingNoVideoTableReturnsSecondsOnly(t *testing.T) {
	// 模型未配置视频价格表时,EstimateBilling 只返回时长系数(退化纯按次),
	// 无 size/time。该用例必须跑在任何 seedVideoPrice 之前
	// (ratio_setting.videoPriceMap 是包级全局状态,测试间共享)。
	c, _ := gin.CreateTestContext(nil)
	req := relaycommon.TaskSubmitReq{Duration: 5, Size: "540p"}
	c.Set("task_request", req)

	a := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{OriginModelName: "viduq3-pro-fast"}
	got := a.EstimateBilling(c, info)
	require.Equal(t, map[string]float64{"seconds": 5}, got)
}

func TestComputeViduRatios(t *testing.T) {
	seedVideoPrice(t)
	peak := beijing("2006-01-02 15:04:05", "2026-09-01 12:00:00")
	offpeak := beijing("2006-01-02 15:04:05", "2026-09-01 23:00:00")

	tests := []struct {
		name   string
		req    relaycommon.TaskSubmitReq
		model  string
		echoed *responsePayload
		now    time.Time
		want   map[string]float64
	}{
		{
			name:  "defaults_pro_720p",
			req:   relaycommon.TaskSubmitReq{},
			model: "viduq3-pro",
			now:   peak,
			want:  map[string]float64{"seconds": 5, "size": 5.0 / 6.0},
		},
		{
			name:  "defaults_turbo_720p",
			req:   relaycommon.TaskSubmitReq{},
			model: "viduq3-turbo",
			now:   peak,
			want:  map[string]float64{"seconds": 5, "size": 12.0 / 13.0},
		},
		{
			name:  "explicit_1080p_no_size_key",
			req:   relaycommon.TaskSubmitReq{Duration: 10, Size: "1080p"},
			model: "viduq3-pro",
			now:   peak,
			want:  map[string]float64{"seconds": 10},
		},
		{
			name:  "turbo_540p",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "540p"},
			model: "viduq3-turbo",
			now:   peak,
			want:  map[string]float64{"seconds": 5, "size": 7.0 / 13.0},
		},
		{
			name:  "pro_540p",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "540p"},
			model: "viduq3-pro",
			now:   peak,
			want:  map[string]float64{"seconds": 5, "size": 3.0 / 8.0},
		},
		{
			name:  "uppercase_resolution_normalized",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "720P"},
			model: "viduq3-pro",
			now:   peak,
			want:  map[string]float64{"seconds": 5, "size": 5.0 / 6.0},
		},
		{
			name:  "duration_saturated_at_max",
			req:   relaycommon.TaskSubmitReq{Duration: 99999},
			model: "viduq3-pro",
			now:   peak,
			want:  map[string]float64{"seconds": relaycommon.MaxTaskDurationSeconds, "size": 5.0 / 6.0},
		},
		{
			name:  "negative_duration_falls_back",
			req:   relaycommon.TaskSubmitReq{Duration: -5},
			model: "viduq3-pro",
			now:   peak,
			want:  map[string]float64{"seconds": 5, "size": 5.0 / 6.0},
		},
		{
			name:  "seconds_string_fallback",
			req:   relaycommon.TaskSubmitReq{Seconds: "8", Size: "540p"},
			model: "viduq3-pro",
			now:   peak,
			want:  map[string]float64{"seconds": 8, "size": 3.0 / 8.0},
		},
		{
			name:  "unknown_resolution_conservative_1",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "4k"},
			model: "viduq3-pro",
			now:   peak,
			want:  map[string]float64{"seconds": 5},
		},
		{
			name:  "unconfigured_model_no_ratios",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "540p"},
			model: "viduq3-pro-fast",
			now:   peak,
			want:  map[string]float64{"seconds": 5},
		},
		{
			name:  "offpeak_pro_720p_half",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			model: "viduq3-pro",
			now:   offpeak,
			want:  map[string]float64{"seconds": 5, "size": 5.0 / 6.0, "time": 0.5},
		},
		{
			name:  "offpeak_pro_540p_not_half",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "540p"},
			model: "viduq3-pro",
			now:   offpeak,
			want:  map[string]float64{"seconds": 5, "size": 3.0 / 8.0, "time": 5.0 / 9.0},
		},
		{
			name:  "offpeak_turbo_1080p_not_half",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "1080p"},
			model: "viduq3-turbo",
			now:   offpeak,
			want:  map[string]float64{"seconds": 5, "time": 7.0 / 13.0},
		},
		{
			name:  "offpeak_turbo_540p_not_half",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "540p"},
			model: "viduq3-turbo",
			now:   offpeak,
			want:  map[string]float64{"seconds": 5, "size": 7.0 / 13.0, "time": 4.0 / 7.0},
		},
		{
			name:  "offpeak_unknown_model_no_discount",
			req:   relaycommon.TaskSubmitReq{Duration: 5, Size: "1080p"},
			model: "viduq3-pro-fast",
			now:   offpeak,
			want:  map[string]float64{"seconds": 5},
		},
		{
			name:   "echo_duration_overrides_request",
			req:    relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			model:  "viduq3-pro",
			echoed: &responsePayload{Duration: 8, Resolution: "1080p"},
			now:    peak,
			want:   map[string]float64{"seconds": 8},
		},
		{
			name:   "echo_resolution_overrides_request",
			req:    relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			model:  "viduq3-turbo",
			echoed: &responsePayload{Duration: 5, Resolution: "540p"},
			now:    peak,
			want:   map[string]float64{"seconds": 5, "size": 7.0 / 13.0},
		},
		{
			name:   "echo_without_actuals_keeps_request",
			req:    relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			model:  "viduq3-pro",
			echoed: &responsePayload{Duration: 0, Resolution: ""},
			now:    peak,
			want:   map[string]float64{"seconds": 5, "size": 5.0 / 6.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeViduRatios(tt.req, tt.model, tt.echoed, tt.now)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEstimateBillingWiring(t *testing.T) {
	seedVideoPrice(t)
	c, _ := gin.CreateTestContext(nil)
	req := relaycommon.TaskSubmitReq{Duration: 5, Size: "540p"}
	c.Set("task_request", req)

	a := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{OriginModelName: "viduq3-turbo"}
	got := a.EstimateBilling(c, info)

	// 与同参数纯函数结果一致(now 取同一时刻,错峰判断自洽)
	want := computeViduRatios(req, "viduq3-turbo", nil, time.Now())
	require.Equal(t, want, got)
	// 请求必须在预扣路径中被缓存,供 AdjustBillingOnSubmit 折算
	require.NotNil(t, a.req)
}

func TestEstimateBillingMissingRequestReturnsNil(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	a := &TaskAdaptor{}
	assert.Nil(t, a.EstimateBilling(c, &relaycommon.RelayInfo{OriginModelName: "viduq3-pro"}))
}

func TestAdjustBillingOnSubmit(t *testing.T) {
	seedVideoPrice(t)
	// 与实现同源的时间快照:错峰时段系数随 now 走,断言两侧始终一致
	now := time.Now()
	model := "viduq3-pro"

	tests := []struct {
		name   string
		req    relaycommon.TaskSubmitReq
		echo   string // 提交响应 JSON
		adjust bool   // 期望触发调整(回显与预扣不一致)
	}{
		{
			name:   "empty_echo_keeps_precharge",
			req:    relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			echo:   `{"task_id":"t1","state":"created"}`,
			adjust: false,
		},
		{
			name:   "echo_matches_precharge_no_churn",
			req:    relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			echo:   `{"task_id":"t1","state":"created","duration":5,"resolution":"720p"}`,
			adjust: false,
		},
		{
			name:   "echo_longer_duration_supplements",
			req:    relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			echo:   `{"task_id":"t1","state":"created","duration":8,"resolution":"720p"}`,
			adjust: true,
		},
		{
			name:   "echo_resolution_supplements",
			req:    relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			echo:   `{"task_id":"t1","state":"created","duration":5,"resolution":"1080p"}`,
			adjust: true,
		},
		{
			name:   "malformed_echo_keeps_precharge",
			req:    relaycommon.TaskSubmitReq{Duration: 5, Size: "720p"},
			echo:   `not-json`,
			adjust: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &TaskAdaptor{req: &tt.req}
			info := &relaycommon.RelayInfo{OriginModelName: model}
			info.PriceData = hosttypes.PriceData{}
			// 预扣阶段入账的系数 = 无回显时同参数的计算结果
			preSet := computeViduRatios(tt.req, model, nil, now)
			for k, v := range preSet {
				info.PriceData.AddOtherRatio(k, v)
			}

			got := a.AdjustBillingOnSubmit(info, []byte(tt.echo))
			if !tt.adjust {
				assert.Nil(t, got)
				return
			}
			var echoed responsePayload
			require.NoError(t, common.Unmarshal([]byte(tt.echo), &echoed))
			assert.Equal(t, computeViduRatios(tt.req, model, &echoed, now), got)
		})
	}
}

func TestAdjustBillingOnCompleteKeepsPrecharge(t *testing.T) {
	// 提交阶段已按回显结算;轮询结果不带时长/分辨率,完成阶段保持预扣金额。
	// 且配置了模型价格的任务按次计费(PerCallBilling),完成结算本身会被跳过。
	a := &TaskAdaptor{}
	assert.Zero(t, a.AdjustBillingOnComplete(nil, nil))
}
