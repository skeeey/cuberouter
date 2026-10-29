package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAttachUpstreamErrorDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "kimi-k3-j",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://modelservice.jdcloud.com",
			UpstreamModelName: "kimi-k3-jy",
		},
	}
	err := &types.NewAPIError{
		Diagnostics: &types.UpstreamErrorDiagnostics{
			StatusCode:  502,
			RequestID:   "jd-request-id",
			BodyPreview: "status_code=502, upstream error",
		},
	}
	other := map[string]interface{}{}

	attachUpstreamErrorDiagnostics(c, other, err, relayInfo)

	diagnostics, ok := other["upstream_error"].(types.UpstreamErrorDiagnostics)
	require.True(t, ok)
	require.Equal(t, 502, diagnostics.StatusCode)
	require.Equal(t, "jd-request-id", diagnostics.RequestID)
	require.Equal(t, "status_code=502, upstream error", diagnostics.BodyPreview)
	require.Equal(t, "https://modelservice.jdcloud.com", diagnostics.Endpoint)
	require.Equal(t, "kimi-k3-j", diagnostics.RequestedModel)
	require.Equal(t, "kimi-k3-jy", diagnostics.UpstreamModel)
	require.Equal(t, "jd-request-id", c.GetString(common.UpstreamRequestIdKey))
}

func TestAttachUpstreamErrorDiagnosticsWithoutDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	other := map[string]interface{}{}

	attachUpstreamErrorDiagnostics(c, other, &types.NewAPIError{}, nil)

	require.Empty(t, other)
	require.Empty(t, c.GetString(common.UpstreamRequestIdKey))
}
