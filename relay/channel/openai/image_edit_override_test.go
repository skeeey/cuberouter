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
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newMultipartEditContext(t *testing.T, model, prompt string, fields map[string]string) *gin.Context {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", model))
	require.NoError(t, writer.WriteField("prompt", prompt))
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}

	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return c
}

func convertAndParseMultipartEdit(t *testing.T, c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) *http.Request {
	t.Helper()

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)
	body, ok := converted.(*bytes.Buffer)
	require.True(t, ok)

	replayed := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	replayed.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	require.NoError(t, replayed.ParseMultipartForm(32<<20))
	return replayed
}

func qwenImageEditOverride(t *testing.T) map[string]interface{} {
	t.Helper()

	var override map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(`{
		"operations": [
			{
				"mode": "set",
				"path": "output_format",
				"value": "png",
				"conditions": [{"path": "model", "mode": "prefix", "value": "qwen-image"}],
				"logic": "AND"
			},
			{
				"mode": "set",
				"path": "response_format",
				"value": "b64_json",
				"conditions": [{"path": "model", "mode": "prefix", "value": "qwen-image"}],
				"logic": "AND"
			}
		]
	}`, &override))
	return override
}

// TestConvertImageEditRequestAppliesParamOverrideToMultipart locks the
// multipart edit path to the same channel param_override contract as JSON
// requests: fields set by matching override operations must be present in the
// re-serialized form without disturbing the uploaded file.
func TestConvertImageEditRequestAppliesParamOverrideToMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const model = "qwen-image-2.1-turbo"
	c := newMultipartEditContext(t, model, "edit this image", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
			ParamOverride:     qwenImageEditOverride(t),
		},
	}

	replayed := convertAndParseMultipartEdit(t, c, info, dto.ImageRequest{
		Model:  model,
		Prompt: "edit this image",
	})

	require.Equal(t, "png", replayed.PostForm.Get("output_format"))
	require.Equal(t, "b64_json", replayed.PostForm.Get("response_format"))
	require.Equal(t, model, replayed.PostForm.Get("model"))
	require.Equal(t, "edit this image", replayed.PostForm.Get("prompt"))
	require.Len(t, replayed.MultipartForm.File["image"], 1)
}

// TestConvertImageEditRequestChannelOverrideWinsOverClientFields locks the
// precedence: channel policy overrides client-supplied form values, matching
// the JSON request path.
func TestConvertImageEditRequestChannelOverrideWinsOverClientFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const model = "qwen-image-2.1-turbo"
	c := newMultipartEditContext(t, model, "edit this image", map[string]string{
		"output_format":   "jpeg",
		"response_format": "url",
	})
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
			ParamOverride:     qwenImageEditOverride(t),
		},
	}

	replayed := convertAndParseMultipartEdit(t, c, info, dto.ImageRequest{
		Model:  model,
		Prompt: "edit this image",
	})

	require.Equal(t, "png", replayed.PostForm.Get("output_format"))
	require.Equal(t, "b64_json", replayed.PostForm.Get("response_format"))
	require.Len(t, replayed.MultipartForm.File["image"], 1)
}

// TestConvertImageEditRequestSkipsNonMatchingParamOverride ensures a channel's
// model-scoped overrides do not leak into other image models served by the
// same channel/type.
func TestConvertImageEditRequestSkipsNonMatchingParamOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const model = "gpt-image-2.5-flare-m"
	c := newMultipartEditContext(t, model, "edit this image", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
			ParamOverride:     qwenImageEditOverride(t),
		},
	}

	replayed := convertAndParseMultipartEdit(t, c, info, dto.ImageRequest{
		Model:  model,
		Prompt: "edit this image",
	})

	require.Empty(t, replayed.PostForm.Get("output_format"))
	require.Empty(t, replayed.PostForm.Get("response_format"))
	require.Equal(t, model, replayed.PostForm.Get("model"))
	require.Len(t, replayed.MultipartForm.File["image"], 1)
}

// TestConvertImageEditRequestWithoutParamOverridePreservesForm keeps the
// existing behavior for channels that do not configure overrides.
func TestConvertImageEditRequestWithoutParamOverridePreservesForm(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const model = "gpt-image-1"
	c := newMultipartEditContext(t, model, "edit this image", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
		},
	}

	replayed := convertAndParseMultipartEdit(t, c, info, dto.ImageRequest{
		Model:  model,
		Prompt: "edit this image",
	})

	require.Equal(t, model, replayed.PostForm.Get("model"))
	require.Equal(t, "edit this image", replayed.PostForm.Get("prompt"))
	require.Empty(t, replayed.PostForm.Get("output_format"))
	require.Len(t, replayed.MultipartForm.File["image"], 1)
}
