package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
)

const adminQuotaDatesMaxRange = 2592000 // 1 month in seconds

// canViewUserDashboard 要求 myRole > target.Role，Root 例外（可查看任何人）。
func canViewUserDashboard(myRole, targetRole int) bool {
	if myRole == common.RoleRootUser {
		return true
	}
	return myRole > targetRole
}

// GetUserQuotaDatesByAdmin GET /api/user/:id/quota-dates?start_timestamp=&end_timestamp=
// 管理员代查指定用户的数据看板。要求 myRole > target.Role（Root 例外）。
func GetUserQuotaDatesByAdmin(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}

	target, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserNotExists)
		return
	}

	myRole := c.GetInt("role")
	if !canViewUserDashboard(myRole, target.Role) {
		common.ApiErrorI18n(c, i18n.MsgAdminCannotViewUserDashboard)
		return
	}

	// Parse failures and negative values must be rejected, not silently
	// coerced to 0; end < start and extreme int64 pairs would otherwise
	// bypass (or overflow) the range check below.
	start, startErr := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, endErr := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startErr != nil || endErr != nil || start < 0 || end < 0 || end < start || end-start > adminQuotaDatesMaxRange {
		common.ApiErrorI18n(c, i18n.MsgAdminQuotaDatesRangeExceeded)
		return
	}

	dates, err := model.GetQuotaDataByUserId(id, start, end)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, dto.UserDashboardPayload{
		User: dto.UserDashboardBrief{
			Id:           target.Id,
			Username:     target.Username,
			DisplayName:  target.DisplayName,
			Role:         target.Role,
			Group:        target.Group,
			Quota:        target.Quota,
			UsedQuota:    target.UsedQuota,
			RequestCount: target.RequestCount,
		},
		Dates: dates,
	})
}
