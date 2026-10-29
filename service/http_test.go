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
package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCaptureUpstreamRequestIdUsesVerifiedHeaderPriority(t *testing.T) {
	// http.Header.Set canonicalizes the name exactly like the HTTP transport
	// does when it parses an upstream response.
	lowercaseSet := http.Header{}
	lowercaseSet.Set("x-request-id", "lower-id")

	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		{"oneapi gateway", http.Header{"X-Oneapi-Request-Id": {"oneapi-id"}}, "oneapi-id"},
		{"astraflow and ali maas", http.Header{"X-Request-Id": {"request-id-value"}}, "request-id-value"},
		{"jd cloud", http.Header{"Request-Id": {"jd-id"}}, "jd-id"},
		{"baidu qianfan", http.Header{"X-Bce-Request-Id": {"bce-id"}}, "bce-id"},
		{"deepseek", http.Header{"X-Ds-Trace-Id": {"ds-id"}}, "ds-id"},
		{"aliyun yike rpc", http.Header{"X-Acs-Request-Id": {"acs-id"}}, "acs-id"},
		{"wire lowercase names", lowercaseSet, "lower-id"},
		{
			"oneapi wins over x-request-id",
			http.Header{"X-Oneapi-Request-Id": {"oneapi"}, "X-Request-Id": {"other"}},
			"oneapi",
		},
		{
			"x-request-id wins over bce",
			http.Header{"X-Request-Id": {"x-request"}, "X-Bce-Request-Id": {"bce"}},
			"x-request",
		},
		{"no request id header", http.Header{"X-Trace-Id": {"trace"}}, ""},
		{"empty header", http.Header{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

			CaptureUpstreamRequestId(ctx, tt.header)

			assert.Equal(t, tt.want, ctx.GetString(common.UpstreamRequestIdKey))
		})
	}
}
