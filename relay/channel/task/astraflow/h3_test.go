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
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsH3Model 锁定精确匹配：MiniMax-H3 同名子串的文本模型与变体不得命中，
// 它们的计费口径（token / 不同价格档）与 -Max/-lite 尚未探测。
func TestIsH3Model(t *testing.T) {
	assert.True(t, isH3Model("MiniMax-H3"))
	assert.True(t, isH3Model("minimax-h3"))
	assert.False(t, isH3Model("minimax-h3-context-ir"))
	assert.False(t, isH3Model("MiniMax-H3-Max"))
	assert.False(t, isH3Model("minimax-h3-lite"))
	assert.False(t, isH3Model("kling-v3"))
}

// TestNormalizeH3Resolution 锁定上游唯一接受的 768P/2K 档位：档位与尺寸按
// 短边归档，保证"发给上游的档位"与计费事实使用同一档位。
func TestNormalizeH3Resolution(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: "768P"},
		{in: "768p", want: "768P"},
		{in: "2k", want: "2K"},
		{in: "720p", want: "768P"},
		{in: "1080p", want: "768P"},
		{in: "1440p", want: "2K"},
		{in: "1920x1080", want: "768P"},
		{in: "2880x1440", want: "2K"},
		{in: "480p", wantErr: true},
		{in: "uhd", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := normalizeH3Resolution(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestH3T2VDefaults 锁定文生视频契约：resolution 缺省 768P、duration 缺省
// 5 秒、ratio 缺省 16:9；计费事实与下发参数取同一份推导。
func TestH3T2VDefaults(t *testing.T) {
	adapter := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{Model: "minimax-h3", Prompt: "a cat on a windowsill"}

	body, facts, err := adapter.deriveH3Request(&req, "MiniMax-H3")
	require.NoError(t, err)

	assert.Equal(t, "MiniMax-H3", body.Model)
	assert.Equal(t, "768P", body.Parameters.Resolution)
	require.NotNil(t, body.Parameters.Duration)
	assert.Equal(t, 5, int(*body.Parameters.Duration))
	assert.Equal(t, "16:9", body.Parameters.Ratio)
	require.Len(t, body.Input.Content, 1)
	assert.Equal(t, "text", body.Input.Content[0].Type)
	require.NotNil(t, body.Input.Content[0].Text)
	assert.Equal(t, "a cat on a windowsill", *body.Input.Content[0].Text)

	assert.Equal(t, map[string]any{
		"seconds":      5.0,
		"resolution":   "768P",
		"input_images": 0.0,
	}, facts)
}

// TestH3T2VRejectsAdaptiveRatio 锁定上游约束：文生视频的 ratio 不能是
// adaptive，显式 adaptive 在预扣费前拒绝而不是执行期失败。
func TestH3T2VRejectsAdaptiveRatio(t *testing.T) {
	adapter := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{Model: "minimax-h3", Prompt: "a cat", Ratio: lo.ToPtr("adaptive")}

	_, _, err := adapter.deriveH3Request(&req, "MiniMax-H3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be adaptive")
}

// TestH3SingleImageBecomesFirstFrame 锁定图生视频契约：单图补 first_frame
// 角色；首帧模式不下发 ratio。
func TestH3SingleImageBecomesFirstFrame(t *testing.T) {
	adapter := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "minimax-h3",
		Prompt: "a girl runs",
		Images: []string{"https://cdn.example.com/first.jpg"},
	}

	body, facts, err := adapter.deriveH3Request(&req, "MiniMax-H3")
	require.NoError(t, err)

	require.Len(t, body.Input.Content, 2)
	image := body.Input.Content[0]
	assert.Equal(t, "image_url", image.Type)
	require.NotNil(t, image.ImageURL)
	require.NotNil(t, image.ImageURL.URL)
	assert.Equal(t, "https://cdn.example.com/first.jpg", *image.ImageURL.URL)
	require.NotNil(t, image.Role)
	assert.Equal(t, "first_frame", *image.Role)
	assert.Empty(t, body.Parameters.Ratio)
	assert.Equal(t, 0.0, facts["input_images"])
}

// TestH3TwoImagesBecomeFrames 锁定双图 → 首尾帧。
func TestH3TwoImagesBecomeFrames(t *testing.T) {
	adapter := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "minimax-h3",
		Prompt: "a transition",
		Images: []string{"https://cdn.example.com/first.jpg", "https://cdn.example.com/last.jpg"},
	}

	body, _, err := adapter.deriveH3Request(&req, "MiniMax-H3")
	require.NoError(t, err)

	require.Len(t, body.Input.Content, 3)
	require.NotNil(t, body.Input.Content[0].Role)
	require.NotNil(t, body.Input.Content[1].Role)
	assert.Equal(t, "first_frame", *body.Input.Content[0].Role)
	assert.Equal(t, "last_frame", *body.Input.Content[1].Role)
	assert.Empty(t, body.Parameters.Ratio)
}

// TestH3ReferenceImagesBillExcess 锁定参考图计费口径：输入图片 5 张以内
// 免费，计费事实只取超出部分（上游 image_input>5 计价）。
func TestH3ReferenceImagesBillExcess(t *testing.T) {
	adapter := &TaskAdaptor{}
	images := []string{
		"https://cdn.example.com/1.jpg",
		"https://cdn.example.com/2.jpg",
		"https://cdn.example.com/3.jpg",
		"https://cdn.example.com/4.jpg",
		"https://cdn.example.com/5.jpg",
		"https://cdn.example.com/6.jpg",
		"https://cdn.example.com/7.jpg",
	}
	req := relaycommon.TaskSubmitReq{Model: "minimax-h3", Prompt: "the same character", Images: images}

	body, facts, err := adapter.deriveH3Request(&req, "MiniMax-H3")
	require.NoError(t, err)

	for i := range images {
		item := body.Input.Content[i]
		require.NotNil(t, item.Role)
		assert.Equal(t, "reference_image", *item.Role)
	}
	assert.Equal(t, 2.0, facts["input_images"])
}

// TestH3ReferenceKeepsExplicitRatio 锁定参考场景：显式合法 ratio 保留，
// adaptive 视为未指定（上游参考场景允许省略）。
func TestH3ReferenceKeepsExplicitRatio(t *testing.T) {
	adapter := &TaskAdaptor{}
	content := []relaycommon.TaskContentItem{
		{
			Type:     "image_url",
			ImageURL: &relaycommon.TaskMediaURL{URL: lo.ToPtr("https://cdn.example.com/ref.jpg")},
			Role:     lo.ToPtr("reference_image"),
		},
	}
	req := relaycommon.TaskSubmitReq{
		Model:   "minimax-h3",
		Prompt:  "a character",
		Content: append(content, relaycommon.TaskContentItem{Type: "text", Text: lo.ToPtr("a character")}),
		Ratio:   lo.ToPtr("9:16"),
	}

	body, _, err := adapter.deriveH3Request(&req, "MiniMax-H3")
	require.NoError(t, err)
	assert.Equal(t, "9:16", body.Parameters.Ratio)

	req.Ratio = lo.ToPtr("adaptive")
	body, _, err = adapter.deriveH3Request(&req, "MiniMax-H3")
	require.NoError(t, err)
	assert.Empty(t, body.Parameters.Ratio)
}

// TestH3DurationBounds 锁定 4~15 秒：越界在预扣费前拒绝。
func TestH3DurationBounds(t *testing.T) {
	adapter := &TaskAdaptor{}
	for _, duration := range []int{3, 16} {
		req := relaycommon.TaskSubmitReq{Model: "minimax-h3", Prompt: "a cat", Duration: duration}
		_, _, err := adapter.deriveH3Request(&req, "MiniMax-H3")
		require.Error(t, err, "duration %d", duration)
	}
	for _, duration := range []int{4, 15} {
		req := relaycommon.TaskSubmitReq{Model: "minimax-h3", Prompt: "a cat", Duration: duration}
		body, facts, err := adapter.deriveH3Request(&req, "MiniMax-H3")
		require.NoError(t, err, "duration %d", duration)
		assert.Equal(t, duration, int(*body.Parameters.Duration))
		assert.Equal(t, float64(duration), facts["seconds"])
	}
}

// TestH3RejectsTooManyImages 锁定参考图上限：超过 9 张在预扣费前拒绝。
func TestH3RejectsTooManyImages(t *testing.T) {
	adapter := &TaskAdaptor{}
	images := make([]string, 10)
	for i := range images {
		images[i] = "https://cdn.example.com/image.jpg"
	}
	req := relaycommon.TaskSubmitReq{Model: "minimax-h3", Prompt: "a cat", Images: images}

	_, _, err := adapter.deriveH3Request(&req, "MiniMax-H3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many input images")
}

// TestH3RejectsMixedFrameAndReferenceMedia 锁定互斥约束：首/尾帧与参考
// 素材混用在预扣费前拒绝（上游不允许两套模式并存）。
func TestH3RejectsMixedFrameAndReferenceMedia(t *testing.T) {
	adapter := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "minimax-h3",
		Prompt: "a cat",
		Content: []relaycommon.TaskContentItem{
			{
				Type:     "image_url",
				ImageURL: &relaycommon.TaskMediaURL{URL: lo.ToPtr("https://cdn.example.com/frame.jpg")},
				Role:     lo.ToPtr("first_frame"),
			},
			{
				Type:     "image_url",
				ImageURL: &relaycommon.TaskMediaURL{URL: lo.ToPtr("https://cdn.example.com/ref.jpg")},
				Role:     lo.ToPtr("reference_image"),
			},
			{Type: "text", Text: lo.ToPtr("a cat")},
		},
	}

	_, _, err := adapter.deriveH3Request(&req, "MiniMax-H3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be mixed")
}

// TestH3UsageSchemaRegistered 锁定计费表达式保存时的 u() 键校验契约。
func TestH3UsageSchemaRegistered(t *testing.T) {
	schema, ok := billingexpr.LookupTaskUsageSchema("MiniMax-H3")
	require.True(t, ok)
	assert.Equal(t, []string{"768P", "2K"}, schema["resolution"].Enum)
	assert.Equal(t, "second", schema["seconds"].Unit)
	assert.Equal(t, "count", schema["input_images"].Unit)
}
