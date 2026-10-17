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
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/go-redis/redis/v8"
)

// userModelAllowListCache is the Redis wire shape of a user's effective member
// model allow list: {"u":true} marks an unrestricted user, while {"m":[...]}
// holds the restricted set, where an empty array means no model is allowed at
// all (distinct from the unrestricted nil list).
type userModelAllowListCache struct {
	Unlimited bool      `json:"u,omitempty"`
	Models    *[]string `json:"m,omitempty"`
}

// userModelAllowListCacheKey returns the cache key of the effective member
// model allow list for a user.
func userModelAllowListCacheKey(userId int) string {
	return fmt.Sprintf("user:model_allowlist:%d", userId)
}

// userModelAllowListCacheVersionKey returns the monotonic fence key of the
// allow-list cache. Invalidation increments it; a refill only writes when the
// fence it captured before reading the database is still current.
func userModelAllowListCacheVersionKey(userId int) string {
	return fmt.Sprintf("user:model_allowlist:ver:%d", userId)
}

// GetUserModelAllowList returns the effective model allow list for a user.
// nil means the user is unrestricted: platform administrators are exempt, a
// user without an active membership in an active organization is unrestricted
// (a disabled or dissolved organization cannot restrict a user), and a
// membership without configured models adds no constraint. A non-nil result is
// the exact set of models the user may use (an empty non-nil list allows
// none), computed as the intersection of the restricted active memberships.
// Redis caches the result, and the database is the authority on a cache miss.
// A database read failure yields nil (fail-open, with a log line) and is never
// cached, so a transient failure cannot pin a restricted user to unrestricted
// until the cache TTL expires.
func GetUserModelAllowList(userId int) []string {
	if IsAdmin(userId) {
		return nil
	}
	key := userModelAllowListCacheKey(userId)
	var cacheVersion int64
	versionLoaded := false
	if common.RedisEnabled {
		if raw, err := common.RedisGet(key); err == nil {
			var cached userModelAllowListCache
			if err := common.Unmarshal([]byte(raw), &cached); err != nil {
				common.SysError("failed to parse user model allow list cache: " + err.Error())
			} else if cached.Unlimited {
				return nil
			} else if cached.Models != nil {
				allow := append([]string{}, (*cached.Models)...)
				sort.Strings(allow)
				return allow
			}
		}
		// Capture the fence before the database snapshot: a refill that read the
		// database before an invalidation must not write its stale value back
		// after it.
		version, err := getUserModelAllowListCacheVersion(userId)
		if err != nil {
			common.SysError("failed to read user model allow list cache version: " + err.Error())
		} else {
			cacheVersion = version
			versionLoaded = true
		}
	}
	allow, loaded := loadMemberModelAllowListFromDB(userId)
	if common.RedisEnabled && loaded && versionLoaded {
		if _, err := cacheUserModelAllowListAtVersion(userId, allow, cacheVersion); err != nil {
			common.SysError("failed to cache user model allow list: " + err.Error())
		}
	}
	return allow
}

// loadMemberModelAllowListFromDB computes the effective allow list from every
// active membership of the user in an active organization. A membership of a
// disabled or dissolved organization imposes no constraint: the organization
// that configured it can no longer govern (a dissolved organization has no
// administrator left), so the restriction must not outlive it. A membership
// whose organization row is missing does not constrain either. A membership
// without configured models imposes no constraint and is skipped; with at
// least one restricted membership the result is the exact-string intersection
// of the restricted rows, sorted. No restricted membership yields nil
// (unrestricted), while a restricted intersection that is empty yields an empty
// non-nil slice, meaning no model is allowed. The second result reports whether
// the database read succeeded; on failure it is false with a nil list, and the
// caller must not cache that nil as "unrestricted".
func loadMemberModelAllowListFromDB(userId int) ([]string, bool) {
	var rows []OrganizationMember
	// The organization status is part of the membership's validity: only
	// memberships governed by an active organization can restrict a user.
	activeOrganizationIds := DB.Model(&Organization{}).Select("id").Where("status = ?", OrganizationStatusActive)
	if err := DB.Select("model_access").
		Where("user_id = ? AND status = ?", userId, OrganizationMemberStatusActive).
		Where("organization_id IN (?)", activeOrganizationIds).
		Find(&rows).Error; err != nil {
		common.SysError("failed to load organization member model access: " + err.Error())
		return nil, false
	}
	restricted := 0
	// counts[name] counts the restricted memberships that allow name; a model
	// is in the intersection when every restricted membership allows it.
	counts := make(map[string]int)
	for _, row := range rows {
		models := ParseMemberModelAccess(row.ModelAccess)
		if len(models) == 0 {
			continue
		}
		restricted++
		for _, name := range models {
			counts[name]++
		}
	}
	if restricted == 0 {
		return nil, true
	}
	allow := make([]string, 0, len(counts))
	for name, count := range counts {
		if count == restricted {
			allow = append(allow, name)
		}
	}
	sort.Strings(allow)
	return allow, true
}

// RefreshUserModelAllowListCache invalidates the cached effective allow list
// for a user. The version bump and the delete are atomic, so a refill that
// captured the previous fence cannot write its snapshot back afterwards.
func RefreshUserModelAllowListCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	const script = `
redis.call('INCR', KEYS[1])
redis.call('DEL', KEYS[2])
return 1`
	return common.RDB.Eval(context.Background(), script,
		[]string{userModelAllowListCacheVersionKey(userId), userModelAllowListCacheKey(userId)},
	).Err()
}

func getUserModelAllowListCacheVersion(userId int) (int64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	raw, err := common.RDB.Get(context.Background(), userModelAllowListCacheVersionKey(userId)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}

// cacheUserModelAllowListAtVersion stores the computed allow list only when the
// fence captured before the database read is still current, so a refill that
// raced an invalidation cannot pin the pre-invalidation value for the TTL.
func cacheUserModelAllowListAtVersion(userId int, allow []string, expectedVersion int64) (bool, error) {
	if !common.RedisEnabled {
		return false, nil
	}
	payload := userModelAllowListCache{}
	if allow == nil {
		payload.Unlimited = true
	} else {
		models := append([]string{}, allow...)
		sort.Strings(models)
		payload.Models = &models
	}
	raw, err := common.Marshal(payload)
	if err != nil {
		return false, err
	}
	const script = `
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
if current ~= tonumber(ARGV[1]) then
  return 0
end
redis.call('SET', KEYS[2], ARGV[2], 'EX', ARGV[3])
return 1`
	result, err := common.RDB.Eval(context.Background(), script,
		[]string{userModelAllowListCacheVersionKey(userId), userModelAllowListCacheKey(userId)},
		expectedVersion, string(raw), userCacheTTLSeconds(),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// IsModelAllowedByMemberAccess reports whether modelName is allowed by an
// effective member allow list. A nil list means no restriction. Entries and
// the looked-up name both match through ratio_setting.FormatMatchingModelName,
// so a normalized entry such as gpt-4-gizmo-* admits the concrete names it
// covers.
func IsModelAllowedByMemberAccess(allow []string, modelName string) bool {
	if allow == nil {
		return true
	}
	allowed := make(map[string]struct{}, 2*len(allow))
	for _, entry := range allow {
		allowed[entry] = struct{}{}
		allowed[ratio_setting.FormatMatchingModelName(entry)] = struct{}{}
	}
	_, ok := allowed[ratio_setting.FormatMatchingModelName(modelName)]
	return ok
}

// FilterModelsByMemberAccess returns the models admitted by an effective
// member allow list, in their input order. A nil list passes models through
// unchanged.
func FilterModelsByMemberAccess(allow []string, models []string) []string {
	if allow == nil {
		return models
	}
	filtered := make([]string, 0, len(models))
	for _, modelName := range models {
		if IsModelAllowedByMemberAccess(allow, modelName) {
			filtered = append(filtered, modelName)
		}
	}
	return filtered
}
