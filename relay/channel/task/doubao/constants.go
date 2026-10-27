package doubao

import (
	"fmt"
	"math"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
)

var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-5-260628",
}

var ChannelName = "doubao-video"

// videoPriceKey 价格表的键：输出分辨率档（is1080p/is4k 均为 false 即 480p/720p 基准档）、输入是否含视频。
type videoPriceKey struct {
	is1080p  bool
	is4k     bool
	hasVideo bool
}

// videoPriceTable 各模型在不同 (输出分辨率档, 是否含视频输入) 下的单价（元/百万 token）。
// 其中零值键 {480p/720p, 不含视频} 为基准价，等于管理员应配置的 ModelRatio；
// 计费时取 实际单价/基准价 作为 OtherRatio。
var videoPriceTable = map[string]map[videoPriceKey]float64{
	"doubao-seedance-2-0-260128": {
		{hasVideo: false}:                46.0,
		{hasVideo: true}:                 28.0,
		{is1080p: true, hasVideo: false}: 51.0,
		{is1080p: true, hasVideo: true}:  31.0,
		{is4k: true, hasVideo: false}:    26.0,
		{is4k: true, hasVideo: true}:     16.0,
	},
	"doubao-seedance-2-0-fast-260128": {
		{hasVideo: false}: 37.0,
		{hasVideo: true}:  22.0,
	},
	"doubao-seedance-2-5-260628": {
		{hasVideo: false}:                70.0,
		{hasVideo: true}:                 42.0,
		{is1080p: true, hasVideo: false}: 77.0,
		{is1080p: true, hasVideo: true}:  46.0,
	},
}

// GetVideoInputRatio 返回指定模型在给定输出分辨率/是否含视频输入下，相对基准价的计费倍率。
// 第二个返回值表示该模型是否配置了价格表；倍率为 1.0 时调用方可忽略该 OtherRatio。
func GetVideoInputRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	prices, ok := videoPriceTable[modelName]
	base := prices[videoPriceKey{}] // 零值键 = {480p/720p, 不含视频} 基准价
	if !ok || base <= 0 {
		return 0, false
	}
	res := normalizeSeedanceResolution(resolution)
	price, ok := prices[videoPriceKey{is1080p: res == "1080p", is4k: res == "4k", hasVideo: hasVideo}]
	if !ok {
		// 未配置的组合（如 fast 无 1080p/4k，上游会自行报错）按基准价计费即可。
		return 1.0, true
	}
	return price / base, true
}

// seedanceCNYPerUSD 与站点定价表达式一致：官方元价 ÷ 7.3 = 计费用的美元价。
const seedanceCNYPerUSD = 7.3

// seedanceTokensPerSecond 各 Seedance 标准模型在输出分辨率档下的视频 token
// 速率（token/秒）。仅用于 tiered_expr 提交时的预扣估算；结算以上游 usage 为准。
// 2.5 速率由实测/按秒价反推；2.0 由官方 5 秒价反推。
var seedanceTokensPerSecond = map[string]map[string]float64{
	"doubao-seedance-2-5-260628": {
		"480p":  9687.4,
		"720p":  21780.5,
		"1080p": 49004.6,
	},
	"doubao-seedance-2-0-260128": {
		"480p":  10043.5,
		"720p":  21608.7,
		"1080p": 48588.2,
		"4k":    194400.0,
	},
}

func isSeedance25Model(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "doubao-seedance-2-5")
}

// isSeedance20Model 只命中标准版 2.0；fast/mini 的 token 单价不同，不能套用同一
// schema 与价格表，留给后续单独接入。
func isSeedance20Model(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "doubao-seedance-2-0") &&
		!strings.Contains(m, "fast") &&
		!strings.Contains(m, "mini")
}

func normalizeSeedanceResolution(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if tier := helper.VideoResolutionTier(s); tier != "" {
		return tier
	}
	return s
}

func validateSeedanceResolution(model, resolution string) error {
	switch resolution {
	case "480p", "720p", "1080p":
		return nil
	case "4k":
		if isSeedance20Model(model) {
			return nil
		}
	}
	return fmt.Errorf("%s: unsupported resolution %q", model, resolution)
}

// videoPriceCNY 返回模型在给定分辨率/视频输入下的官方 token 单价（元/百万 token）。
func videoPriceCNY(modelName, resolution string, hasVideo bool) (float64, bool) {
	prices, ok := videoPriceTable[modelName]
	if !ok {
		return 0, false
	}
	res := normalizeSeedanceResolution(resolution)
	price, ok := prices[videoPriceKey{is1080p: res == "1080p", is4k: res == "4k", hasVideo: hasVideo}]
	if !ok || price <= 0 {
		return 0, false
	}
	return price, true
}

func seedanceUnitPriceUSD(modelName, resolution string, hasVideo bool) (float64, bool) {
	cny, ok := videoPriceCNY(modelName, resolution, hasVideo)
	if !ok {
		return 0, false
	}
	return cny / seedanceCNYPerUSD, true
}

func estimateSeedanceTokens(modelName, resolution string, seconds int) (float64, bool) {
	rates, ok := seedanceTokensPerSecond[modelName]
	if !ok {
		return 0, false
	}
	rate, ok := rates[normalizeSeedanceResolution(resolution)]
	if !ok || rate <= 0 {
		return 0, false
	}
	if seconds <= 0 {
		seconds = 5
	}
	if seconds > relaycommon.MaxTaskDurationSeconds {
		seconds = relaycommon.MaxTaskDurationSeconds
	}
	return math.Ceil(rate * float64(seconds)), true
}
