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
package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpBreakdownTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/breakdown.db"), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}))
	t.Cleanup(func() {
		WaitForQuotaCacheWorkers()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedisEnabled
	})
	return db
}

// TestFillUserTopUpBreakdown 锁定管理端「现金 / 赠金」口径：
// 现金只算成功的在线充值订单（按订单汇率快照折算），赠金为余额与历史已消费
// 之和扣除现金后的余额；待支付订单与其他用户的订单都不能计入。
func TestFillUserTopUpBreakdown(t *testing.T) {
	origType := operation_setting.GetQuotaDisplayType()
	origQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() {
		operation_setting.SetQuotaDisplayTypeForTest(origType)
		common.QuotaPerUnit = origQuotaPerUnit
	})
	operation_setting.SetQuotaDisplayTypeForTest(operation_setting.QuotaDisplayTypeUSD)
	common.QuotaPerUnit = 500000

	db := setupTopUpBreakdownTestDB(t)

	user := User{
		Username: "breakdown-user", Password: "password", Group: "default",
		AffCode: "breakdown-aff", Quota: 400000, UsedQuota: 200000,
	}
	require.NoError(t, db.Create(&user).Error)
	other := User{
		Username: "breakdown-other", Password: "password", Group: "default",
		AffCode: "breakdown-other-aff",
	}
	require.NoError(t, db.Create(&other).Error)

	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, Amount: 1, TradeNo: "breakdown-success",
		PaymentMethod: "payment", PaymentProvider: PaymentProviderEpay,
		Status: common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, Amount: 99, TradeNo: "breakdown-pending",
		PaymentMethod: "payment", PaymentProvider: PaymentProviderEpay,
		Status: common.TopUpStatusPending,
	}).Error)
	require.NoError(t, db.Create(&TopUp{
		UserId: other.Id, Amount: 5, TradeNo: "breakdown-other-success",
		PaymentMethod: "payment", PaymentProvider: PaymentProviderEpay,
		Status: common.TopUpStatusSuccess,
	}).Error)
	// 旧系统管理员手动充值：保留在充值记录里，但不计入现金。
	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, Amount: 99999999, Money: 1460,
		TradeNo: "admin_balance:1:breakdown", PaymentMethod: PaymentMethodAdminRecharge,
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess,
	}).Error)

	FillUserTopUpBreakdown(&user)
	require.NotNil(t, user.CashTopUpTotal)
	require.NotNil(t, user.GiftTopUpTotal)
	assert.Equal(t, 500000, *user.CashTopUpTotal)
	assert.Equal(t, 100000, *user.GiftTopUpTotal)

	clamped := User{
		Username: "breakdown-clamped", Password: "password", Group: "default",
		AffCode: "breakdown-clamped-aff", Quota: 100000,
	}
	require.NoError(t, db.Create(&clamped).Error)
	require.NoError(t, db.Create(&TopUp{
		UserId: clamped.Id, Amount: 1, TradeNo: "breakdown-clamped-success",
		PaymentMethod: "payment", PaymentProvider: PaymentProviderEpay,
		Status: common.TopUpStatusSuccess,
	}).Error)
	FillUserTopUpBreakdown(&clamped)
	require.NotNil(t, clamped.CashTopUpTotal)
	require.NotNil(t, clamped.GiftTopUpTotal)
	assert.Equal(t, 500000, *clamped.CashTopUpTotal)
	assert.Equal(t, 0, *clamped.GiftTopUpTotal)

	operation_setting.SetQuotaDisplayTypeForTest(operation_setting.QuotaDisplayTypeCNY)
	cnyUser := User{
		Username: "breakdown-cny", Password: "password", Group: "default",
		AffCode: "breakdown-cny-aff", Quota: 700000,
	}
	require.NoError(t, db.Create(&cnyUser).Error)
	require.NoError(t, db.Create(&TopUp{
		UserId: cnyUser.Id, Amount: 10, Rate: 7.3, TradeNo: "breakdown-cny-success",
		PaymentMethod: "payment", PaymentProvider: PaymentProviderEpay,
		Status: common.TopUpStatusSuccess,
	}).Error)
	FillUserTopUpBreakdown(&cnyUser)
	// 10 元 ÷ 7.3 × 500,000 ≈ 684,931.5068 → 684,932
	require.NotNil(t, cnyUser.CashTopUpTotal)
	assert.Equal(t, 684932, *cnyUser.CashTopUpTotal)

	// 旧系统迁移订单：Amount 存的是 quota（10 元 → 684,932），不是元。
	legacyCny := User{
		Username: "breakdown-legacy-cny", Password: "password", Group: "default",
		AffCode: "breakdown-legacy-cny-aff", Quota: 700000,
	}
	require.NoError(t, db.Create(&legacyCny).Error)
	require.NoError(t, db.Create(&TopUp{
		UserId: legacyCny.Id, Amount: 684932, Money: 10, TradeNo: "breakdown-legacy-cny",
		PaymentMethod: "payment", PaymentProvider: PaymentProviderEpay,
		Status: common.TopUpStatusSuccess,
	}).Error)
	FillUserTopUpBreakdown(&legacyCny)
	require.NotNil(t, legacyCny.CashTopUpTotal)
	assert.Equal(t, 684932, *legacyCny.CashTopUpTotal)

	manualCash := User{
		Username: "breakdown-manual-cash", Password: "password", Group: "default",
		AffCode: "breakdown-manual-cash-aff", Quota: 400000, UsedQuota: 200000,
		ManualCashTopUpTotal: 100000,
	}
	require.NoError(t, db.Create(&manualCash).Error)
	FillUserTopUpBreakdown(&manualCash)
	require.NotNil(t, manualCash.CashTopUpTotal)
	require.NotNil(t, manualCash.GiftTopUpTotal)
	assert.Equal(t, 100000, *manualCash.CashTopUpTotal)
	assert.Equal(t, 500000, *manualCash.GiftTopUpTotal)
}

func TestIncreaseUserQuotaAndManualCash(t *testing.T) {
	db := setupTopUpBreakdownTestDB(t)
	user := User{
		Username: "manual-cash-increment", Password: "password", Group: "default",
		AffCode: "manual-cash-increment-aff", Quota: 100000,
	}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, IncreaseUserQuotaAndManualCash(user.Id, 50000))
	var updated User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, 150000, updated.Quota)
	assert.EqualValues(t, 50000, updated.ManualCashTopUpTotal)

	require.NoError(t, IncreaseUserQuotaAndManualCash(user.Id, 25000))
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, 175000, updated.Quota)
	assert.EqualValues(t, 75000, updated.ManualCashTopUpTotal)
}
