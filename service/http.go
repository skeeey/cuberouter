package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// upstreamRequestIdHeaders lists upstream response headers that carry a
// per-request ID, in priority order. Only headers verified against the live
// upstreams behind cuberouter channels belong here.
//
// TODO(upstream-request-id): providers not covered yet — IDs that only appear
// in the response body (Zhipu bigmodel `request_id`, Baidu AIGC eos-sg
// `requestId`), and upstreams not yet probed (Vidu, fncompute, BoosterAI,
// MOSS audio, DaoCloud, internal MOSS/Qwen-Image workers, AWS LiteLLM).
var upstreamRequestIdHeaders = []string{
	common.RequestIdKey, // new-api / cuberouter gateways, e.g. computrix
	"X-Request-Id",      // AstraFlow, Ali MaaS, Volcengine Ark, 牛耳
	"Request-Id",        // JD Cloud
	"X-Bce-Request-Id",  // Baidu Qianfan
	"X-Ds-Trace-Id",     // DeepSeek
	"X-Acs-Request-Id",  // Aliyun Yike RPC
}

// CaptureUpstreamRequestId records the highest-priority upstream request ID
// header on the Gin context so the consume/task log can persist it.
func CaptureUpstreamRequestId(c *gin.Context, header http.Header) {
	if c == nil || header == nil {
		return
	}
	for _, name := range upstreamRequestIdHeaders {
		if id := header.Get(name); id != "" {
			c.Set(common.UpstreamRequestIdKey, id)
			return
		}
	}
}

// ShouldCopyUpstreamHeader checks whether a given upstream response header
// should be copied to the client response. It returns false for Content-Length
// (managed separately) and X-Oneapi-Request-Id (to preserve the local instance
// ID). When the upstream header is X-Oneapi-Request-Id, the value is captured
// into the Gin context for later logging.
func ShouldCopyUpstreamHeader(c *gin.Context, k string, v []string) bool {
	if strings.EqualFold(k, "Content-Length") {
		return false
	}
	if strings.EqualFold(k, common.RequestIdKey) {
		if c != nil && len(v) > 0 {
			c.Set(common.UpstreamRequestIdKey, v[0])
		}
		return false
	}
	return true
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		for k, v := range src.Header {
			if !ShouldCopyUpstreamHeader(c, k, v) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}
