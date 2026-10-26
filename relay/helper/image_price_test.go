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

package helper

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedImagePrice 写入图片按张价格表(固定画质档位)。
// 档位价均为精确二进制小数(0.0625=1/16, 0.125=1/8),
// 与 QuotaPerUnit(500000) 相乘无浮点截断误差,断言可精确到整数 quota。
func seedImagePrice(t *testing.T) {
	t.Helper()
	require.NoError(t, ratio_setting.UpdateImagePriceByJSONString(`{
		"img-price-model": {"rows": [
			{"tier": "fast", "price": 0.0625},
			{"tier": "high", "price": 0.125}
		]}
	}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateImagePriceByJSONString(""))
	})
}

func TestModelPriceHelperImagePriceTable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	seedImagePrice(t)

	newInfo := func(model string) (*gin.Context, *relaycommon.RelayInfo) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("group", "default")
		return ctx, &relaycommon.RelayInfo{
			OriginModelName: model,
			UserGroup:       "default",
			UsingGroup:      "default",
		}
	}

	t.Run("tier and count ratios multiply anchor price", func(t *testing.T) {
		ctx, info := newInfo("img-price-model")
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 2},
			ImageQuality:  "fast",
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		require.True(t, priceData.UsePrice)
		// 锚点 0.125 × 500000 × n=2 × tier=0.5 = 62500
		require.Equal(t, 62500, priceData.QuotaToPreConsume)
		assert.Equal(t, float64(2), priceData.OtherRatios()["n"])
		assert.Equal(t, 0.5, priceData.OtherRatios()["quality"])
		// 结算复用同一 PriceData:OtherRatios 与 info.PriceData 一致
		assert.Equal(t, priceData.OtherRatios(), info.PriceData.OtherRatios())
	})

	t.Run("anchor tier row carries no tier ratio", func(t *testing.T) {
		ctx, info := newInfo("img-price-model")
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 1},
			ImageQuality:  "high",
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		require.Equal(t, 62500, priceData.QuotaToPreConsume)
		assert.False(t, priceData.HasOtherRatio("quality"))
	})

	t.Run("unknown quality bills at anchor", func(t *testing.T) {
		ctx, info := newInfo("img-price-model")
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 2},
			ImageQuality:  "2048x2048",
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		// 锚点 0.125 × 500000 × n=2 = 125000
		require.Equal(t, 125000, priceData.QuotaToPreConsume)
		assert.False(t, priceData.HasOtherRatio("quality"))
	})

	t.Run("normalized tier matches table row", func(t *testing.T) {
		ctx, info := newInfo("img-price-model")
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 1},
			ImageQuality:  " FAST ",
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		require.Equal(t, 31250, priceData.QuotaToPreConsume)
		assert.Equal(t, 0.5, priceData.OtherRatios()["quality"])
	})

	t.Run("legacy ImagePriceRatio is not stacked on the table", func(t *testing.T) {
		ctx, info := newInfo("img-price-model")
		meta := &types.TokenCountMeta{
			BillingRatios:   map[string]float64{"n": 1},
			ImageQuality:    "high",
			ImagePriceRatio: 2,
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		// 若 ImagePriceRatio 被叠加,结果会是 125000
		require.Equal(t, 62500, priceData.QuotaToPreConsume)
	})

	t.Run("missing quality bills at anchor", func(t *testing.T) {
		ctx, info := newInfo("img-price-model")
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 1},
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		require.Equal(t, 62500, priceData.QuotaToPreConsume)
		assert.False(t, priceData.HasOtherRatio("quality"))
	})
}

func TestHasModelBillingConfigImagePriceTable(t *testing.T) {
	seedImagePrice(t)
	// 仅配置图片价格表(无 ModelPrice/ModelRatio)也算有计费配置
	assert.True(t, HasModelBillingConfig("img-price-model"))
	assert.False(t, HasModelBillingConfig("img-unknown-model"))
}

// seedImagePriceResolution 写入分辨率价格表，价格与 MOSS 方案 A 一致。
func seedImagePriceResolution(t *testing.T) {
	t.Helper()
	require.NoError(t, ratio_setting.UpdateImagePriceByJSONString(`{
		"img-resolution-model": {
			"default_resolution": "1024x1024",
			"rows": [
				{"resolution": "512x512", "price": 0.002},
				{"resolution": "1024x1024", "price": 0.006},
				{"resolution": "1536x1024", "price": 0.01},
				{"resolution": "2048x2048", "price": 0.038}
			]
		}
	}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateImagePriceByJSONString(""))
	})
}

func TestModelPriceHelperImagePriceResolutionTable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	seedImagePriceResolution(t)

	newInfo := func() (*gin.Context, *relaycommon.RelayInfo) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("group", "default")
		return ctx, &relaycommon.RelayInfo{
			OriginModelName: "img-resolution-model",
			UserGroup:       "default",
			UsingGroup:      "default",
		}
	}

	t.Run("resolution and count ratios multiply anchor price", func(t *testing.T) {
		ctx, info := newInfo()
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 2},
			ImageSize:     "1024x1024",
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		require.True(t, priceData.UsePrice)
		// 锚点 0.038 × 500000 × n=2 × (0.006/0.038) = 6000
		require.Equal(t, 6000, priceData.QuotaToPreConsume)
		assert.Equal(t, float64(2), priceData.OtherRatios()["n"])
		assert.InDelta(t, 0.006/0.038, priceData.OtherRatios()["size"], 1e-15)
	})

	t.Run("missing size uses default resolution", func(t *testing.T) {
		ctx, info := newInfo()
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 1},
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		require.Equal(t, 3000, priceData.QuotaToPreConsume)
		assert.InDelta(t, 0.006/0.038, priceData.OtherRatios()["size"], 1e-15)
	})

	t.Run("largest resolution bills at anchor", func(t *testing.T) {
		ctx, info := newInfo()
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 1},
			ImageSize:     "2048x2048",
		}
		priceData, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.NoError(t, err)
		require.Equal(t, 19000, priceData.QuotaToPreConsume)
		assert.False(t, priceData.HasOtherRatio("size"))
	})

	t.Run("invalid size format is rejected instead of defaulting", func(t *testing.T) {
		ctx, info := newInfo()
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 1},
			ImageSize:     "auto",
		}
		_, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.Error(t, err)
		require.ErrorContains(t, err, "auto")
	})

	t.Run("unsupported size is rejected with supported list", func(t *testing.T) {
		ctx, info := newInfo()
		meta := &types.TokenCountMeta{
			BillingRatios: map[string]float64{"n": 1},
			ImageSize:     "999x999",
		}
		_, err := ModelPriceHelper(ctx, info, 1000, meta)
		require.Error(t, err)
		require.ErrorContains(t, err, "999x999")
		require.ErrorContains(t, err, "512x512")
		require.ErrorContains(t, err, "2048x2048")
	})
}
