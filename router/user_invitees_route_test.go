package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// 路由顺序回归：GET /api/user/:id/invitees 与 /:id/quota-dates 必须命中各自
// handler，不能被 /api/user/:id 通配截获（Gin radix tree 对两者共存处理正确，
// 两种注册顺序均断言正确路由）。
func TestUserSubRouteOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	run := func(t *testing.T, idFirst bool) {
		r := gin.New()
		api := r.Group("/api")
		user := api.Group("/user")
		admin := user.Group("/")
		{
			admin.GET("/search", func(c *gin.Context) { c.String(200, "search") })
			if idFirst {
				admin.GET("/:id", func(c *gin.Context) { c.String(200, "id="+c.Param("id")) })
				admin.GET("/:id/invitees", func(c *gin.Context) { c.String(200, "invitees="+c.Param("id")) })
				admin.GET("/:id/quota-dates", func(c *gin.Context) { c.String(200, "quota-dates="+c.Param("id")) })
			} else {
				admin.GET("/:id/invitees", func(c *gin.Context) { c.String(200, "invitees="+c.Param("id")) })
				admin.GET("/:id/quota-dates", func(c *gin.Context) { c.String(200, "quota-dates="+c.Param("id")) })
				admin.GET("/:id", func(c *gin.Context) { c.String(200, "id="+c.Param("id")) })
			}
			admin.GET("/2fa/stats", func(c *gin.Context) { c.String(200, "stats") })
		}

		cases := []struct {
			path string
			want string
			code int
		}{
			{"/api/user/1", "id=1", 200},
			{"/api/user/42/invitees", "invitees=42", 200},
			{"/api/user/42/quota-dates", "quota-dates=42", 200},
			{"/api/user/search", "search", 200},
			{"/api/user/2fa/stats", "stats", 200},
		}
		for _, c := range cases {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, c.code, w.Code, "GET %s", c.path)
			assert.Equal(t, c.want, w.Body.String(), "GET %s", c.path)
		}
	}

	t.Run("id-first (other registration order)", func(t *testing.T) { run(t, true) })
	t.Run("subroutes-first (current production order)", func(t *testing.T) { run(t, false) })
}
