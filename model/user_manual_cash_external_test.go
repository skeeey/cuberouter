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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// testManualCashTopUpFlow 覆盖现金手动充值的落库与拆分口径：
// 现金累计同时包含手动现金充值和管理员之外的成功在线充值订单。
func testManualCashTopUpFlow(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousRedisEnabled := common.RedisEnabled
	DB, LOG_DB = db, db
	common.RedisEnabled = false
	t.Cleanup(func() {
		WaitForQuotaCacheWorkers()
		DB, LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
	})
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}))

	origType := operation_setting.GetQuotaDisplayType()
	origQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() {
		operation_setting.SetQuotaDisplayTypeForTest(origType)
		common.QuotaPerUnit = origQuotaPerUnit
	})
	operation_setting.SetQuotaDisplayTypeForTest(operation_setting.QuotaDisplayTypeUSD)
	common.QuotaPerUnit = 500000

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	user := User{
		Username: "manual-cash-flow-" + suffix, Password: "password",
		Group: "default", AffCode: "manual-cash-flow-" + suffix, Quota: 100000,
	}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, IncreaseUserQuotaAndManualCash(user.Id, 50000))
	var updated User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, 150000, updated.Quota)
	assert.EqualValues(t, 50000, updated.ManualCashTopUpTotal)

	// 模拟一笔在线充值：额度入账 + 成功订单；现金应包含 50000 手动 + 500000 在线。
	require.NoError(t, IncreaseUserQuota(user.Id, 500000, true))
	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, Amount: 1, TradeNo: "manual-cash-flow-" + suffix,
		PaymentMethod: "payment", PaymentProvider: PaymentProviderEpay,
		Status: common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, db.First(&updated, user.Id).Error)
	FillUserTopUpBreakdown(&updated)
	require.NotNil(t, updated.CashTopUpTotal)
	require.NotNil(t, updated.GiftTopUpTotal)
	assert.Equal(t, 550000, *updated.CashTopUpTotal)
	assert.Equal(t, 100000, *updated.GiftTopUpTotal)
}

func TestManualCashTopUpFlowSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/manual-cash-flow.db"), &gorm.Config{})
	require.NoError(t, err)
	testManualCashTopUpFlow(t, db)
}

func TestManualCashTopUpFlowMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	testManualCashTopUpFlow(t, mysqlScratchDatabase(t, dsn))
}

func TestManualCashTopUpFlowPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	testManualCashTopUpFlow(t, postgresScratchSchema(t, dsn))
}
