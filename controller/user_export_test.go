package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests must not trigger i18n.Init(): it is a process-wide sync.Once
// that permanently swaps common.TranslateMessage to i18n.T, and this file
// sorts alphabetically before user_manage_test.go (raw i18n keys in response
// bodies) and user_ops_test.go (first to initialize the bundle). Because the
// bundle is therefore never loaded while these tests run, i18n.T would panic;
// the handler and expectations use common.TranslateMessage, which main.go's
// i18n.Init() resolves to i18n.T in production but falls back to returning
// the raw key (the behavior user_manage_test.go depends on) before that.
func TestFormatUserRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/export", nil)

	u := &model.User{
		Id:           42,
		Username:     "alice",
		DisplayName:  "Alice A.",
		Role:         common.RoleAdminUser,
		Status:       common.UserStatusEnabled,
		Group:        "vip",
		Quota:        1000,
		UsedQuota:    250,
		RequestCount: 17,
		CreatedAt:    time.Date(2026, 5, 1, 12, 30, 45, 0, time.Local).Unix(),
		Remark:       "test remark",
		AffCode:      "AF42",
		AffCount:     3,
		InviterId:    7,
	}
	got := formatUserRow(c, u)
	want := []string{
		"42", "alice", "Alice A.", common.TranslateMessage(c, i18n.MsgAdminRoleAdmin), common.TranslateMessage(c, i18n.MsgAdminStatusEnabled), "vip",
		"1000", "250", "17",
		"2026-05-01 12:30:45", "test remark", "AF42", "3", "7",
	}
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i], got[i], "col %d", i)
	}
}

func TestFormatUserRowZeroCreatedAtAndOpsRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/export", nil)

	u := &model.User{Id: 1, Username: "u", Role: common.RoleOpsUser, Status: common.UserStatusDisabled, CreatedAt: 0}
	got := formatUserRow(c, u)
	assert.Equal(t, "", got[9], "created_at col for zero value")
	assert.Equal(t, common.TranslateMessage(c, i18n.MsgAdminRoleOps), got[3], "ops role gets a real label, not Unknown")
	assert.Equal(t, common.TranslateMessage(c, i18n.MsgAdminStatusDisabled), got[4])
}

// setupManageUserTestDB instead of setupOpsUserTestDB: the ops helper calls
// i18n.Init(), a process-wide sync.Once that permanently swaps
// common.TranslateMessage to i18n.T. Tests in this package that run later
// assert raw i18n keys in responses (user_manage_test.go) or depend on being
// the first to initialize the bundle (user_ops_test.go), and this file sorts
// alphabetically before user_ops_test.go, so it must not trigger that
// initialization.
func TestExportUsersWritesCsvByIds(t *testing.T) {
	db := setupManageUserTestDB(t)
	a := model.User{Username: "export-a", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "export-a-aff"}
	b := model.User{Username: "export-b", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "export-b-aff"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/export", strings.NewReader(
		fmt.Sprintf(`{"ids":[%d,%d],"keyword":"export-a","format":"csv"}`, a.Id, b.Id)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("username", "admin-1")
	ExportUsers(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/csv; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "users_")
	raw := recorder.Body.Bytes()
	assert.True(t, len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF, "must start with UTF-8 BOM")
	rows, err := csv.NewReader(strings.NewReader(string(raw[3:]))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 3) // header + the two requested users (ids win over keyword)
	assert.Equal(t, userExportHeaders(c), rows[0])
	// The model returns rows in DB order, not requested-id order, and the CSV
	// is documented as order-agnostic (progress.md, Task 3 deferred minor), so
	// the data rows are checked by membership rather than by position.
	var usernames []string
	for _, r := range rows[1:] {
		usernames = append(usernames, r[1])
	}
	assert.ElementsMatch(t, []string{"export-a", "export-b"}, usernames)
	assert.NotContains(t, recorder.Body.String(), "password", "password must never appear in the CSV")
}

func TestExportUsersWritesCsvByFilter(t *testing.T) {
	db := setupManageUserTestDB(t)
	for _, name := range []string{"filter-one", "filter-two", "other-user"} {
		require.NoError(t, db.Create(&model.User{Username: name, Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: name + "-aff"}).Error)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/export", strings.NewReader(`{"keyword":"filter-","format":"csv"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("username", "admin-1")
	ExportUsers(c)

	raw := recorder.Body.Bytes()
	rows, err := csv.NewReader(strings.NewReader(string(raw[3:]))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 3) // header + filter-one + filter-two
	// SearchUsers defaults to ORDER BY id DESC (NewUserSortOptions with no
	// sort), so rows come back in DB order — reverse insertion order here.
	// The CSV is documented as order-agnostic (progress.md, Task 3 deferred
	// minor), so the data rows are checked by membership rather than position.
	var usernames []string
	for _, r := range rows[1:] {
		usernames = append(usernames, r[1])
	}
	assert.ElementsMatch(t, []string{"filter-one", "filter-two"}, usernames)
}

// A failed batch surfaces as an error response instead of a successful
// (partial) CSV: all batches are fetched before the header is written.
func TestExportUsersFailsWithoutWritingCsvOnDbError(t *testing.T) {
	db := setupManageUserTestDB(t)
	u := model.User{Username: "export-fail", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "export-fail-aff"}
	require.NoError(t, db.Create(&u).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/export", strings.NewReader(fmt.Sprintf(`{"ids":[%d],"format":"csv"}`, u.Id)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("username", "admin-1")
	ExportUsers(c)

	var body struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.False(t, body.Success)
	raw := recorder.Body.Bytes()
	assert.False(t, len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF, "must not write a CSV on export failure")
}

func TestExportUsersCsvNeutralizesFormulas(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Username: "=2+5", DisplayName: "+SUM(A1:A2)", Password: "password",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "@default", AffCode: "-1+1", Remark: "=HYPERLINK(\"http://x\")",
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/export", strings.NewReader(`{"keyword":"2+5","format":"csv"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("username", "admin-1")
	ExportUsers(c)

	raw := recorder.Body.Bytes()
	require.GreaterOrEqual(t, len(raw), 3)
	rows, err := csv.NewReader(strings.NewReader(string(raw[3:]))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2) // header + the formula-flagged user
	assert.Equal(t, "'=2+5", rows[1][1])
	assert.Equal(t, "'+SUM(A1:A2)", rows[1][2])
	assert.Equal(t, "'@default", rows[1][5])
	assert.Equal(t, "'=HYPERLINK(\"http://x\")", rows[1][10])
	assert.Equal(t, "'-1+1", rows[1][11])
}

func TestExportUsersRejectsUnsupportedFormat(t *testing.T) {
	setupManageUserTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/export", strings.NewReader(`{"format":"xlsx"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("username", "admin-1")
	ExportUsers(c)

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.Equal(t, common.TranslateMessage(c, i18n.MsgAdminExportUnsupportedFormat, map[string]any{"Format": "xlsx"}), body.Message)
}

func TestExportUsersRejectsMalformedJson(t *testing.T) {
	setupManageUserTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/export", strings.NewReader(`{"ids":`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("username", "admin-1")
	ExportUsers(c)

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.Equal(t, common.TranslateMessage(c, i18n.MsgInvalidParams), body.Message)
}

// An oversized ids list is rejected with batch_too_many before any query runs.
func TestExportUsersRejectsTooManyIds(t *testing.T) {
	setupManageUserTestDB(t)
	gin.SetMode(gin.TestMode)

	var b strings.Builder
	b.WriteString(`{"ids":[`)
	for i := 1; i <= maxExportIds+1; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%d", i)
	}
	b.WriteString(`]}`)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/export", strings.NewReader(b.String()))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("username", "admin-1")
	ExportUsers(c)

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.Equal(t, common.TranslateMessage(c, i18n.MsgBatchTooMany, map[string]any{"Max": maxExportIds}), body.Message)
}
