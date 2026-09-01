package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetUserInvitees scopes purely by inviter_id and orders newest-first.
func TestGetUserInviteesScopesAndOrders(t *testing.T) {
	truncateTables(t)
	// target A: two invitees (newer first); unrelated inviter B: one invitee (must never leak)
	a := insertOpsUser(t, "admin-view-a", 0)
	b := insertOpsUser(t, "admin-view-b", 0)
	old := insertOpsUser(t, "invitee-old", a.Id, func(u *User) { u.CreatedAt = 1000 })
	new := insertOpsUser(t, "invitee-new", a.Id, func(u *User) {
		u.CreatedAt = 2000
		u.SetAccessToken("invitee-token")
	})
	insertOpsUser(t, "invitee-of-b", b.Id)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	users, total, err := GetUserInvitees(a.Id, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, users, 2)
	assert.Equal(t, new.Id, users[0].Id, "newest first")
	assert.Equal(t, old.Id, users[1].Id)
	for _, u := range users {
		assert.Equal(t, "", u.Password, "password must be omitted")
		assert.Nil(t, u.AccessToken, "access token must be omitted")
	}
}

// Pagination respects PageInfo offset/limit.
func TestGetUserInviteesPagination(t *testing.T) {
	truncateTables(t)
	inviter := insertOpsUser(t, "admin-paged", 0)
	created := make([]*User, 0, 5)
	for i := 0; i < 5; i++ {
		created = append(created, insertOpsUser(t, "paged-"+string(rune('a'+i)), inviter.Id,
			func(u *User) { u.CreatedAt = int64(1000 + i) }))
	}

	page1 := &common.PageInfo{Page: 1, PageSize: 2}
	users, total, err := GetUserInvitees(inviter.Id, page1)
	require.NoError(t, err)
	assert.EqualValues(t, 5, total)
	require.Len(t, users, 2)
	assert.Equal(t, created[4].Id, users[0].Id)

	page3 := &common.PageInfo{Page: 3, PageSize: 2}
	users, _, err = GetUserInvitees(inviter.Id, page3)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, created[0].Id, users[0].Id)
}

// Zero invitees -> total 0, empty slice (never nil).
func TestGetUserInviteesEmpty(t *testing.T) {
	truncateTables(t)
	users, total, err := GetUserInvitees(123456, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 0, total)
	require.NotNil(t, users)
	assert.Empty(t, users)
}

// No role-based scoping at model level: the target's role (admin/root) is
// irrelevant — scoping is purely inviter_id.
func TestGetUserInviteesIgnoresTargetRole(t *testing.T) {
	truncateTables(t)
	root := insertOpsUser(t, "admin-root-target", 0, func(u *User) { u.Role = common.RoleRootUser })
	insertOpsUser(t, "root-invitee", root.Id)

	users, total, err := GetUserInvitees(root.Id, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, users, 1)
	assert.Equal(t, "root-invitee", users[0].Username)
}
