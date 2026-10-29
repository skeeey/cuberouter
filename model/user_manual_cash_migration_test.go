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

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// testManualCashTopUpColumnMigration simulates upgrading a users table created
// before manual_cash_topup_total existed: the column is dropped, AutoMigrate
// must add it back with 0 for existing rows, and running it again must be a
// no-op.
func testManualCashTopUpColumnMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&User{}))

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	user := User{
		Username:             "legacy-manual-cash-" + suffix,
		Password:             "password",
		Group:                "default",
		AffCode:              "legacy-manual-cash-aff-" + suffix,
		Quota:                123456,
		ManualCashTopUpTotal: 999,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Migrator().DropColumn(&User{}, "manual_cash_topup_total"))
	require.False(t, db.Migrator().HasColumn(&User{}, "manual_cash_topup_total"))

	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.AutoMigrate(&User{}))
	require.True(t, db.Migrator().HasColumn(&User{}, "manual_cash_topup_total"))

	var preserved User
	require.NoError(t, db.Where("id = ?", user.Id).First(&preserved).Error)
	assert.Equal(t, 123456, preserved.Quota)
	assert.Zero(t, preserved.ManualCashTopUpTotal)
}

func TestMigrateUserManualCashTopUpColumnSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/manual-cash-migration.db"), &gorm.Config{})
	require.NoError(t, err)
	testManualCashTopUpColumnMigration(t, db)
}

func TestMigrateUserManualCashTopUpColumnMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	testManualCashTopUpColumnMigration(t, db)
}

func TestMigrateUserManualCashTopUpColumnPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	testManualCashTopUpColumnMigration(t, db)
}
