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

package plugins_test

import (
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const yikeBaseURL = "https://yike.cn-shanghai.aliyuncs.com"

func loadAlibabaPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("alibaba")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "alibaba"})
	require.NoError(t, err)
	return plugin
}

func callAlibabaHook(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	return decoded
}

func callAlibabaHookRaw(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	return value
}

func callAlibabaPath(t *testing.T, plugin *jsplugin.LoadedPlugin, path []string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", path, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	return decoded
}

func TestAlibabaYikeTextToVideoSubmitRequest(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	descriptor := callAlibabaHook(t, plugin, "buildSubmitRequest", map[string]any{
		"baseUrl":       yikeBaseURL,
		"apiKey":        "test-key",
		"upstreamModel": "wan3.0-video",
		"model":         "seedance-2.0-a",
		"requestBody": map[string]any{
			"model":    "seedance-2.0-a",
			"prompt":   "a cat",
			"size":     "1280x720",
			"duration": float64(5),
		},
	})

	require.Equal(t, "POST", descriptor["method"])
	require.Equal(t, "text_to_video", descriptor["action"])
	headers, ok := descriptor["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bearer test-key", headers["Authorization"])
	assert.Equal(t, "application/json", headers["Content-Type"])

	rawURL, _ := descriptor["url"].(string)
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "yike.cn-shanghai.aliyuncs.com", parsed.Host)
	assert.Equal(t, "/", parsed.Path)

	query := parsed.Query()
	assert.Equal(t, "SubmitVideoGenerationJob", query.Get("Action"))
	assert.Equal(t, "2026-07-07", query.Get("Version"))
	assert.Equal(t, "json", query.Get("Format"))
	assert.NotEmpty(t, query.Get("Timestamp"))
	assert.Equal(t, "text_to_video", query.Get("JobType"))
	assert.Equal(t, "wan3.0-video", query.Get("Model"))
	assert.Equal(t, "general", query.Get("Scene"))
	assert.Equal(t, "720P", query.Get("Resolution"))
	assert.Equal(t, "16:9", query.Get("AspectRatio"))
	assert.Equal(t, "5", query.Get("Duration"))
	assert.Equal(t, "1", query.Get("N"))

	var input map[string]any
	require.NoError(t, common.Unmarshal([]byte(query.Get("Input")), &input))
	assert.Equal(t, "a cat", input["Prompt"])
	assert.NotContains(t, input, "Medias")
}

func TestAlibabaYikeImageToVideoSubmitRequest(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	descriptor := callAlibabaHook(t, plugin, "buildSubmitRequest", map[string]any{
		"baseUrl":       yikeBaseURL,
		"apiKey":        "test-key",
		"upstreamModel": "wan3.0-video",
		"model":         "seedance-2.0-a",
		"requestBody": map[string]any{
			"model":    "seedance-2.0-a",
			"prompt":   "the cat starts walking",
			"image":    "https://cdn.example/first.png",
			"size":     "720P",
			"duration": float64(4),
		},
	})

	require.Equal(t, "image_to_video", descriptor["action"])
	rawURL, _ := descriptor["url"].(string)
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	query := parsed.Query()
	assert.Equal(t, "image_to_video", query.Get("JobType"))

	var input struct {
		Prompt string `json:"Prompt"`
		Medias []struct {
			Type string `json:"Type"`
			URL  string `json:"Url"`
		} `json:"Medias"`
	}
	require.NoError(t, common.Unmarshal([]byte(query.Get("Input")), &input))
	assert.Equal(t, "the cat starts walking", input.Prompt)
	require.Len(t, input.Medias, 1)
	assert.Equal(t, "image", input.Medias[0].Type)
	assert.Equal(t, "https://cdn.example/first.png", input.Medias[0].URL)
}

func TestAlibabaYikeParseSubmitResponse(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	value := callAlibabaHook(t, plugin, "parseSubmitResponse",
		map[string]any{"baseUrl": yikeBaseURL, "apiKey": "test-key"},
		map[string]any{
			"statusCode": float64(200),
			"headers":    map[string]any{},
			"body":       map[string]any{"RequestId": "req-1", "JobId": "ag_123"},
		},
	)

	assert.Equal(t, "ag_123", value["taskId"])
	taskData, ok := value["taskData"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ag_123", taskData["JobId"])

	t.Run("success with message still returns job id", func(t *testing.T) {
		value := callAlibabaHook(t, plugin, "parseSubmitResponse",
			map[string]any{"baseUrl": yikeBaseURL, "apiKey": "test-key"},
			map[string]any{
				"statusCode": float64(200),
				"headers":    map[string]any{},
				"body":       map[string]any{"RequestId": "req-1", "JobId": "ag_456", "Message": "accepted"},
			},
		)
		assert.Equal(t, "ag_456", value["taskId"])
	})
}

func TestAlibabaYikeBuildQueryRequest(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	descriptor := callAlibabaHook(t, plugin, "buildQueryRequest", map[string]any{
		"baseUrl": yikeBaseURL,
		"apiKey":  "test-key",
		"taskId":  "ag_123",
	})

	assert.Equal(t, "GET", descriptor["method"])
	headers, ok := descriptor["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bearer test-key", headers["Authorization"])

	rawURL, _ := descriptor["url"].(string)
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	assert.Equal(t, yikeBaseURL, parsed.Scheme+"://"+parsed.Host)
	query := parsed.Query()
	assert.Equal(t, "GetVideoGenerationJob", query.Get("Action"))
	assert.Equal(t, "2026-07-07", query.Get("Version"))
	assert.Equal(t, "json", query.Get("Format"))
	assert.NotEmpty(t, query.Get("Timestamp"))
	assert.Equal(t, "ag_123", query.Get("JobId"))
}

func TestAlibabaYikeParseTaskResult(t *testing.T) {
	plugin := loadAlibabaPlugin(t)
	ctx := map[string]any{"baseUrl": yikeBaseURL}

	t.Run("executing", func(t *testing.T) {
		value := callAlibabaHook(t, plugin, "parseTaskResult", ctx, map[string]any{
			"VideoGenerationJob": map[string]any{"Status": "Executing"},
		})
		assert.Equal(t, "IN_PROGRESS", value["status"])
	})

	t.Run("finished", func(t *testing.T) {
		value := callAlibabaHook(t, plugin, "parseTaskResult", ctx, map[string]any{
			"VideoGenerationJob": map[string]any{
				"Status": "Finished",
				"Output": `{"Medias":[{"OutputUrl":"https://oss.example/video.mp4","MediaId":"m1"}]}`,
			},
		})
		assert.Equal(t, "SUCCESS", value["status"])
		assert.Equal(t, "https://oss.example/video.mp4", value["url"])
	})

	t.Run("failed", func(t *testing.T) {
		value := callAlibabaHook(t, plugin, "parseTaskResult", ctx, map[string]any{
			"VideoGenerationJob": map[string]any{
				"Status":       "Failed",
				"ErrorMessage": "The service encountered an unexpected error.",
			},
		})
		assert.Equal(t, "FAILURE", value["status"])
		assert.Contains(t, value["reason"], "unexpected error")
	})

	t.Run("empty response stays unknown for retry", func(t *testing.T) {
		value := callAlibabaHook(t, plugin, "parseTaskResult", ctx, map[string]any{})
		assert.Equal(t, "UNKNOWN", value["status"])
	})

	t.Run("explicit yike error fails", func(t *testing.T) {
		value := callAlibabaHook(t, plugin, "parseTaskResult", ctx, map[string]any{
			"Code":    "JobIdNotExist",
			"Message": "Job not found",
		})
		assert.Equal(t, "FAILURE", value["status"])
		assert.Contains(t, value["reason"], "JobIdNotExist")
	})
}

func TestAlibabaYikeExtractUsage(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	value := callAlibabaHook(t, plugin, "extractUsage", map[string]any{
		"baseUrl":       yikeBaseURL,
		"upstreamModel": "wan3.0-video",
		"usagePurpose":  "facts",
		"requestBody": map[string]any{
			"model":    "seedance-2.0-a",
			"prompt":   "a cat",
			"size":     "1920x1080",
			"duration": float64(6),
		},
	})

	assert.Equal(t, float64(6), value["seconds"])
	assert.Equal(t, "1080P", value["resolution"])
}

func TestAlibabaYikeExtractUsageOnComplete(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	value := callAlibabaHook(t, plugin, "extractUsageOnComplete",
		map[string]any{"task_id": "task-1", "status": "SUCCESS"},
		map[string]any{"taskId": "ag_123", "status": "SUCCESS"},
		map[string]any{
			"VideoGenerationJob": map[string]any{
				"Status":     "Finished",
				"Duration":   "8",
				"Resolution": "720P",
			},
		},
	)

	assert.Equal(t, float64(8), value["seconds"])
	assert.Equal(t, "720P", value["resolution"])
}

func TestAlibabaYikeVideoArtifacts(t *testing.T) {
	plugin := loadAlibabaPlugin(t)
	taskData := map[string]any{
		"VideoGenerationJob": map[string]any{
			"Status": "Finished",
			"Output": `{"Medias":[{"OutputUrl":"https://oss.example/video.mp4","MediaId":"m1"}]}`,
		},
	}

	t.Run("lists video artifact", func(t *testing.T) {
		raw := callAlibabaHookRaw(t, plugin, "listArtifacts", map[string]any{
			"task_id": "task-1",
			"status":  "SUCCESS",
			"data":    taskData,
		})
		encoded, err := common.Marshal(raw)
		require.NoError(t, err)
		var artifacts []map[string]any
		require.NoError(t, common.Unmarshal(encoded, &artifacts))
		require.Len(t, artifacts, 1)
		assert.Equal(t, "video", artifacts[0]["key"])
		assert.Equal(t, "video", artifacts[0]["type"])
	})

	t.Run("builds credentialless content request", func(t *testing.T) {
		descriptor := callAlibabaHook(t, plugin, "buildContentRequest", map[string]any{
			"data":           taskData,
			"artifactKey":    "video",
			"clientRequest":  map[string]any{"method": "GET"},
			"baseUrl":        yikeBaseURL,
			"apiKey":         "test-key",
			"upstreamTaskId": "ag_123",
		})
		assert.Equal(t, "https://oss.example/video.mp4", descriptor["url"])
		assert.Equal(t, "GET", descriptor["method"])
		assert.Equal(t, true, descriptor["credentialless"])
	})
}

func TestAlibabaYikeOpenAIVideoRender(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	value := callAlibabaPath(t, plugin, []string{"openai_video", "render"},
		map[string]any{"protocol": "openai_video", "operation": "retrieve"},
		map[string]any{
			"task_id":    "task-1",
			"status":     "SUCCESS",
			"created_at": float64(10),
			"updated_at": float64(20),
			"data": map[string]any{
				"VideoGenerationJob": map[string]any{"Status": "Finished"},
			},
		},
	)

	assert.Equal(t, "task-1", value["id"])
	assert.Equal(t, "completed", value["status"])
	assert.Equal(t, "video", value["object"])
}

func TestAlibabaYikeRejectsMetadataOnlyBillingFields(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl":       yikeBaseURL,
		"apiKey":        "test-key",
		"upstreamModel": "wan3.0-video",
		"requestBody": map[string]any{
			"model":  "seedance-2.0-a",
			"prompt": "a cat",
			"metadata": map[string]any{
				"resolution": "1080P",
			},
		},
	})

	require.ErrorContains(t, err, "metadata.resolution")
}

func TestAlibabaYikeSizeMatchesBillingTier(t *testing.T) {
	plugin := loadAlibabaPlugin(t)
	ctx := func(size string) map[string]any {
		return map[string]any{
			"baseUrl":       yikeBaseURL,
			"apiKey":        "test-key",
			"upstreamModel": "wan3.0-video",
			"requestBody": map[string]any{
				"model":  "seedance-2.0-a",
				"prompt": "a cat",
				"size":   size,
			},
		}
	}

	t.Run("maps 832x480 to 480P", func(t *testing.T) {
		descriptor := callAlibabaHook(t, plugin, "buildSubmitRequest", ctx("832x480"))
		parsed, err := url.Parse(descriptor["url"].(string))
		require.NoError(t, err)
		query := parsed.Query()
		assert.Equal(t, "480P", query.Get("Resolution"))
		assert.Equal(t, "16:9", query.Get("AspectRatio"))
	})

	t.Run("rejects 960x540 because billing has no 540p row", func(t *testing.T) {
		_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx("960x540"))
		require.ErrorContains(t, err, "540P")
	})

	t.Run("accepts adaptive ratio for wan3 output", func(t *testing.T) {
		descriptor := callAlibabaHook(t, plugin, "buildSubmitRequest", map[string]any{
			"baseUrl":       yikeBaseURL,
			"apiKey":        "test-key",
			"upstreamModel": "wan3.0-video-prime",
			"requestBody": map[string]any{
				"model":  "seedance-2.5-a",
				"prompt": "a cat",
				"size":   "720P",
				"ratio":  "adaptive",
			},
		})
		parsed, err := url.Parse(descriptor["url"].(string))
		require.NoError(t, err)
		assert.Equal(t, "adaptive", parsed.Query().Get("AspectRatio"))
	})
}

func TestAlibabaYikeOwnsStationModelsWithoutAstraFlowNameCollision(t *testing.T) {
	plugin := loadAlibabaPlugin(t)
	assert.Contains(t, plugin.Meta.Models, "seedance-2.0-a")
	assert.Contains(t, plugin.Meta.Models, "seedance-2.5-a")
	assert.NotContains(t, plugin.Meta.Models, "wan3.0-video")
	assert.NotContains(t, plugin.Meta.Models, "wan3.0-video-prime")
}

func TestAlibabaYikeRejectsMultipleOutputs(t *testing.T) {
	plugin := loadAlibabaPlugin(t)

	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl":       yikeBaseURL,
		"apiKey":        "test-key",
		"upstreamModel": "wan3.0-video",
		"requestBody": map[string]any{
			"model":  "seedance-2.0-a",
			"prompt": "a cat",
			"metadata": map[string]any{
				"n": 2,
			},
		},
	})

	require.ErrorContains(t, err, "n=1")
}
