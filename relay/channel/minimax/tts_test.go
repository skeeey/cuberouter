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
package minimax

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertAudioRequestUsesHexOutputFormat 锁定 t2a_v2 的响应编码契约：
// output_format 只接受 hex/url，客户端请求的音频格式属于 audio_setting.format。
// 把 mp3 之类写进 output_format 会被上游以 base_resp 2013 拒绝（HTTP 仍是 200）。
func TestConvertAudioRequestUsesHexOutputFormat(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeAudioSpeech,
		OriginModelName: "speech-02-hd",
	}
	request := dto.AudioRequest{
		Model:          "speech-02-hd",
		Input:          "你好",
		Voice:          "female-shaonv",
		ResponseFormat: "mp3",
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	reader, err := adaptor.ConvertAudioRequest(c, info, request)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Equal(t, "hex", payload["output_format"])
	audioSetting, ok := payload["audio_setting"].(map[string]any)
	require.True(t, ok, "audio_setting must be an object")
	assert.Equal(t, "mp3", audioSetting["format"])
}
