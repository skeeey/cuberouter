package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminDashboardMessages verifies the admin dashboard messages render with
// complete per-locale text (no unrendered placeholders) through the same path
// the app uses (i18n.Translate, which backs common.ApiErrorI18n).
func TestAdminDashboardMessages(t *testing.T) {
	require.NoError(t, Init())

	cases := []struct {
		name string
		lang string
		key  string
		want string
	}{
		{name: "en cannot view", lang: LangEn, key: MsgAdminCannotViewUserDashboard, want: "No permission to view this user's data dashboard"},
		{name: "zh-CN cannot view", lang: LangZhCN, key: MsgAdminCannotViewUserDashboard, want: "无权查看该用户的数据看板"},
		{name: "zh-TW range", lang: LangZhTW, key: MsgAdminQuotaDatesRangeExceeded, want: "時間跨度不能超過 1 個月"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Translate(tc.lang, tc.key)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestAdminExportHeaderAndLabelMessages verifies the admin CSV export headers,
// role/status labels, and the unsupported-format message render per locale
// (they back userExportHeaders and formatUserRow, which must never leak
// untranslated keys).
func TestAdminExportHeaderAndLabelMessages(t *testing.T) {
	require.NoError(t, Init())

	cases := []struct {
		name string
		lang string
		key  string
		args map[string]any
		want string
	}{
		{name: "en header role", lang: LangEn, key: MsgAdminExportHeaderRole, want: "Role"},
		{name: "zh-CN header remark", lang: LangZhCN, key: MsgAdminExportHeaderRemark, want: "备注"},
		{name: "zh-TW header inviter", lang: LangZhTW, key: MsgAdminExportHeaderInviterId, want: "邀請人 ID"},
		{name: "en role ops", lang: LangEn, key: MsgAdminRoleOps, want: "Ops"},
		{name: "zh-CN role ops", lang: LangZhCN, key: MsgAdminRoleOps, want: "运营"},
		{name: "zh-TW status disabled", lang: LangZhTW, key: MsgAdminStatusDisabled, want: "已停用"},
		{name: "en unsupported format", lang: LangEn, key: MsgAdminExportUnsupportedFormat, args: map[string]any{"Format": "xlsx"}, want: "Unsupported export format: xlsx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Translate(tc.lang, tc.key, tc.args)
			assert.Equal(t, tc.want, got)
		})
	}
}
