package controller

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
)

// userExportHeaders are the CSV column headers for the admin user export,
// resolved per request locale. The order is stable for downstream scripts.
func userExportHeaders(c *gin.Context) []string {
	return []string{
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderId),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderUsername),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderDisplayName),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderRole),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderStatus),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderGroup),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderQuota),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderUsedQuota),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderRequestCount),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderCreatedAt),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderRemark),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderAffCode),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderAffCount),
		common.TranslateMessage(c, i18n.MsgAdminExportHeaderInviterId),
	}
}

// formatUserRow maps a User to one CSV row, with role/status labels resolved
// for the request locale. User-controlled cells are neutralized against
// spreadsheet formula injection via csvSafeCell.
func formatUserRow(c *gin.Context, u *model.User) []string {
	createdAt := ""
	if u.CreatedAt > 0 {
		createdAt = time.Unix(u.CreatedAt, 0).Format("2006-01-02 15:04:05")
	}
	role := fmt.Sprintf("%s(%d)", common.TranslateMessage(c, i18n.MsgAdminRoleUnknown), u.Role)
	switch u.Role {
	case common.RoleCommonUser:
		role = common.TranslateMessage(c, i18n.MsgAdminRoleCommon)
	case common.RoleOpsUser:
		role = common.TranslateMessage(c, i18n.MsgAdminRoleOps)
	case common.RoleAdminUser:
		role = common.TranslateMessage(c, i18n.MsgAdminRoleAdmin)
	case common.RoleRootUser:
		role = common.TranslateMessage(c, i18n.MsgAdminRoleRoot)
	}
	status := fmt.Sprintf("%s(%d)", common.TranslateMessage(c, i18n.MsgAdminStatusUnknown), u.Status)
	switch u.Status {
	case common.UserStatusEnabled:
		status = common.TranslateMessage(c, i18n.MsgAdminStatusEnabled)
	case common.UserStatusDisabled:
		status = common.TranslateMessage(c, i18n.MsgAdminStatusDisabled)
	}
	return []string{
		strconv.Itoa(u.Id),
		csvSafeCell(u.Username),
		csvSafeCell(u.DisplayName),
		role,
		status,
		csvSafeCell(u.Group),
		strconv.Itoa(u.Quota),
		strconv.Itoa(u.UsedQuota),
		strconv.Itoa(u.RequestCount),
		createdAt,
		csvSafeCell(u.Remark),
		csvSafeCell(u.AffCode),
		strconv.Itoa(u.AffCount),
		strconv.Itoa(u.InviterId),
	}
}

// ExportUsersRequest is the body for the admin user CSV export.
type ExportUsersRequest struct {
	Ids     []int  `json:"ids"`
	Keyword string `json:"keyword"`
	Group   string `json:"group"`
	Format  string `json:"format"`
}

// Export bounds: an unbounded ids list or a filter matching the whole user
// table would materialize the complete result set on the request path.
const (
	maxExportIds  = 10000
	maxExportRows = 50000
)

// ExportUsers streams the full user table as a CSV file. Already AdminAuth'd.
// The ids take precedence over the keyword/group filter when both are given.
func ExportUsers(c *gin.Context) {
	var req ExportUsersRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.Format != "" && req.Format != "csv" {
		common.ApiErrorI18n(c, i18n.MsgAdminExportUnsupportedFormat, map[string]any{"Format": req.Format})
		return
	}
	if len(req.Ids) > maxExportIds {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": maxExportIds})
		return
	}

	adminId := c.GetInt("id")
	adminName := c.GetString("username")

	// Fetch and validate every batch before writing the CSV header, so a
	// failed batch fails the request instead of looking like a successful
	// (partial) export.
	var users []*model.User
	var err error
	var mode string
	if len(req.Ids) > 0 {
		mode = "selected"
		users, err = model.ExportUsersByIds(req.Ids)
	} else {
		mode = "filter"
		users, err = model.ExportUsersByFilter(req.Keyword, req.Group, maxExportRows)
	}
	if err != nil {
		if errors.Is(err, model.ErrExportRowsExceeded) {
			common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": maxExportRows})
			return
		}
		common.ApiError(c, err)
		return
	}

	// ASCII filename: non-ASCII Content-Disposition filenames render as
	// garbage in browsers regardless of charset hints.
	filename := fmt.Sprintf("users_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	// UTF-8 BOM so Excel opens the CSV as UTF-8
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	_ = w.Write(userExportHeaders(c))

	for _, u := range users {
		_ = w.Write(formatUserRow(c, u))
	}
	w.Flush()

	common.SysLog(fmt.Sprintf(
		"admin %d (%s) exported users count=%d mode=%s",
		adminId, adminName, len(users), mode,
	))
}
