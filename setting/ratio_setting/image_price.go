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

package ratio_setting

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

// ImagePriceTiers 是图片按张价目表的固定画质档位(与图像请求的 quality 字段取值一致)。
var ImagePriceTiers = []string{"fast", "standard", "high"}

// DefaultImagePriceResolution 是分辨率价目表未声明 default_resolution 时的默认值，
// 与当前自建图像上游的默认输出尺寸保持一致。
const DefaultImagePriceResolution = "1024x1024"

var imagePriceResolutionPattern = regexp.MustCompile(`^[1-9][0-9]*x[1-9][0-9]*$`)

// ImagePriceRow 是图片按张价目表的一行。
// 画质表用 Tier(fast/standard/high)，分辨率表用 Resolution(WxH)；两者互斥。
// Price 为该档位/分辨率下单张图片的美元价格(与视频表同口径,USD 存储)。
type ImagePriceRow struct {
	Tier       string  `json:"tier,omitempty"`
	Resolution string  `json:"resolution,omitempty"`
	Price      float64 `json:"price"`
}

// ImagePriceTable 是单个模型的图片按张价目表。
// 画质表:画质档位 → 单张美元价；分辨率表:分辨率 → 单张美元价。
// 计费 = 锚点(最高价行)× 档位/分辨率系数 × 张数 n。
// 画质表表外 quality 按锚点(最贵档)计费；分辨率表 size 缺省时用
// DefaultResolution，表外 size 由调用方拒绝。
type ImagePriceTable struct {
	DefaultResolution string          `json:"default_resolution,omitempty"`
	Rows              []ImagePriceRow `json:"rows"`
}

var imagePriceMap = types.NewRWMap[string, *ImagePriceTable]()

// ParseImagePriceMap 解析 ImagePrice option 的顶层 map(模型名 → 价格表)。
func ParseImagePriceMap(jsonStr string) (map[string]*ImagePriceTable, error) {
	m := make(map[string]*ImagePriceTable)
	if strings.TrimSpace(jsonStr) == "" {
		return m, nil
	}
	if err := common.UnmarshalJsonStr(jsonStr, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func UpdateImagePriceByJSONString(jsonStr string) error {
	m, err := ParseImagePriceMap(jsonStr)
	if err != nil {
		return err
	}
	for model, table := range m {
		if table == nil || len(table.Rows) == 0 {
			return fmt.Errorf("image price table for %s is empty", model)
		}
		resolutionBased := NormalizeImagePriceResolution(table.Rows[0].Resolution) != ""
		if !resolutionBased && strings.TrimSpace(table.Rows[0].Tier) == "" {
			return fmt.Errorf("image price table for %s has neither tier nor resolution", model)
		}
		if !resolutionBased && strings.TrimSpace(table.DefaultResolution) != "" {
			return fmt.Errorf("quality image price table for %s must not set default_resolution", model)
		}
		if resolutionBased {
			if rawDefault := strings.TrimSpace(table.DefaultResolution); rawDefault != "" {
				table.DefaultResolution = NormalizeImagePriceResolution(rawDefault)
				if table.DefaultResolution == "" {
					return fmt.Errorf("invalid default image price resolution for %s: %q (want WIDTHxHEIGHT)",
						model, rawDefault)
				}
			}
		}
		seen := make(map[string]struct{}, len(table.Rows))
		for i := range table.Rows {
			row := &table.Rows[i]
			if resolutionBased {
				if strings.TrimSpace(row.Tier) != "" {
					return fmt.Errorf("image price row for %s mixes tier and resolution: %+v", model, row)
				}
				row.Resolution = NormalizeImagePriceResolution(row.Resolution)
				if row.Resolution == "" {
					return fmt.Errorf("invalid image price resolution for %s: %q (want WIDTHxHEIGHT)",
						model, row.Resolution)
				}
				if _, dup := seen[row.Resolution]; dup {
					return fmt.Errorf("duplicate image price resolution for %s: %q", model, row.Resolution)
				}
				seen[row.Resolution] = struct{}{}
			} else {
				if strings.TrimSpace(row.Resolution) != "" {
					return fmt.Errorf("image price row for %s mixes tier and resolution: %+v", model, row)
				}
				row.Tier = NormalizeImagePriceTier(row.Tier)
				if !IsValidImagePriceTier(row.Tier) {
					return fmt.Errorf("invalid image price tier for %s: %q (want one of %v)",
						model, row.Tier, ImagePriceTiers)
				}
				if _, dup := seen[row.Tier]; dup {
					return fmt.Errorf("duplicate image price tier for %s: %q", model, row.Tier)
				}
				seen[row.Tier] = struct{}{}
			}
			if row.Price <= 0 {
				return fmt.Errorf("invalid image price row for %s: %+v", model, row)
			}
		}
		if resolutionBased && table.DefaultResolution != "" {
			if _, ok := seen[table.DefaultResolution]; !ok {
				return fmt.Errorf("default image price resolution for %s is not in rows: %q",
					model, table.DefaultResolution)
			}
		}
	}
	// 校验全部通过后整体替换;失败时保持旧配置不变。
	// ReplaceAll 单次加锁原子交换,读方不会观察到空表中间态。
	imagePriceMap.ReplaceAll(m)
	return nil
}

func GetImagePrice(model string) (*ImagePriceTable, bool) {
	return imagePriceMap.Get(model)
}

// NormalizeImagePriceTier 归一化画质档位:去首尾空白、小写。
func NormalizeImagePriceTier(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// IsValidImagePriceTier 判断 tier 是否为固定画质档位之一(调用前需已归一化或传入规范值)。
func IsValidImagePriceTier(tier string) bool {
	for _, t := range ImagePriceTiers {
		if tier == t {
			return true
		}
	}
	return false
}

// ImagePriceAnchor 返回表内最高价行作为锚点(保证档位系数 ≤ 1)。
// 画质表表外档位按锚点(最贵档)计费,与视频按秒表的保守口径一致；
// 分辨率表的表外 size 由调用方拒绝,不会落到锚点。
func ImagePriceAnchor(t *ImagePriceTable) float64 {
	anchor := 0.0
	for _, row := range t.Rows {
		if row.Price > anchor {
			anchor = row.Price
		}
	}
	return anchor
}

// ImagePriceTierRatio 返回给定画质档位的计费系数(档位价 ÷ 锚点,≤ 1)。
// 档位未配置(或为空)时返回 0,调用方不加系数,即按锚点计费。
func ImagePriceTierRatio(t *ImagePriceTable, tier string) float64 {
	anchor := ImagePriceAnchor(t)
	if anchor <= 0 {
		return 0
	}
	key := NormalizeImagePriceTier(tier)
	if key == "" {
		return 0
	}
	for _, row := range t.Rows {
		if row.Tier == key {
			return row.Price / anchor
		}
	}
	return 0
}

// NormalizeImagePriceResolution 归一化分辨率:去首尾空白、小写，并校验 WIDTHxHEIGHT
// (宽高均为正整数)。格式不合法时返回空串。
func NormalizeImagePriceResolution(resolution string) string {
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if !imagePriceResolutionPattern.MatchString(resolution) {
		return ""
	}
	return resolution
}

// IsResolutionBased 判断价格表是否为分辨率表(校验通过后所有行模式一致)。
func (t *ImagePriceTable) IsResolutionBased() bool {
	return t != nil && len(t.Rows) > 0 && strings.TrimSpace(t.Rows[0].Resolution) != ""
}

// EffectiveDefaultResolution 返回分辨率表的默认分辨率；未配置时使用上游默认的
// 1024x1024，保证客户端不传 size 时预扣与上游实际输出一致。
func (t *ImagePriceTable) EffectiveDefaultResolution() string {
	if t == nil {
		return DefaultImagePriceResolution
	}
	if resolution := NormalizeImagePriceResolution(t.DefaultResolution); resolution != "" {
		return resolution
	}
	return DefaultImagePriceResolution
}

// ImagePriceResolutionRatio 返回给定分辨率的计费系数(分辨率价 ÷ 锚点,≤ 1)。
// 分辨率不在表内时 ok=false，调用方应拒绝请求而不是静默按锚点收费。
func ImagePriceResolutionRatio(t *ImagePriceTable, resolution string) (float64, bool) {
	anchor := ImagePriceAnchor(t)
	if anchor <= 0 {
		return 0, false
	}
	key := NormalizeImagePriceResolution(resolution)
	if key == "" {
		return 0, false
	}
	for _, row := range t.Rows {
		if row.Resolution == key {
			return row.Price / anchor, true
		}
	}
	return 0, false
}

// SupportedImagePriceResolutions 返回分辨率表内配置的分辨率，用于错误提示。
func SupportedImagePriceResolutions(t *ImagePriceTable) []string {
	if t == nil {
		return nil
	}
	resolutions := make([]string, 0, len(t.Rows))
	for _, row := range t.Rows {
		if row.Resolution != "" {
			resolutions = append(resolutions, row.Resolution)
		}
	}
	return resolutions
}
