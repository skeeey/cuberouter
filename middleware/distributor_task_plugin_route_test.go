package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 供应商无关的任务端点（POST /v1/video/generations）允许 Task Plugin（62 类）渠道
// 按渠道自身设置里的 task_plugin_key 参与选择；其它路由保持"必须先有插件身份"的
// 约束，插件渠道不会捡走普通中继流量。
func TestDistributeTaskPluginChannelOnVendorNeutralVideoEndpoint(t *testing.T) {
	require.NoError(t, appI18n.Init())

	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousCache := common.MemoryCacheEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Task{}, &model.Channel{}, &model.Ability{}))
	model.DB = database
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	// 走内存缓存选择路径（与线上一致），它同样应用渠道过滤器
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.MemoryCacheEnabled = previousCache
	})

	setting := `{"task_plugin_key":"kokoni"}`
	channel := &model.Channel{
		Name:    "kokoni",
		Key:     "sk-test",
		Status:  common.ChannelStatusEnabled,
		Type:    constant.ChannelTypeTaskPlugin,
		Models:  "moworld-t2v",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, channel.Insert())
	// 走内存缓存选择路径（与线上一致），它会应用同样的过滤器
	model.InitChannelCache()

	distribute := func(t *testing.T, path string, vendorNeutral bool) (*httptest.ResponseRecorder, *gin.Context) {
		t.Helper()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"moworld-t2v"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		if vendorNeutral {
			common.SetContextKey(c, constant.ContextKeyTaskPluginChannelAllowed, true)
		}
		Distribute()(c)
		return recorder, c
	}

	t.Run("vendor neutral endpoint selects the task plugin channel", func(t *testing.T) {
		recorder, c := distribute(t, "/v1/video/generations", true)
		require.False(t, c.IsAborted(), recorder.Body.String())
		assert.Equal(t, channel.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
		assert.Equal(t, "kokoni", c.GetString("task_plugin_key"))
	})

	t.Run("ordinary relay route keeps rejecting the task plugin channel", func(t *testing.T) {
		recorder, c := distribute(t, "/v1/chat/completions", false)
		require.True(t, c.IsAborted())
		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "model_not_found")
	})
}
