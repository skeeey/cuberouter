# Admin User Management Extensions (管理员用户管理扩展)

> Last Updated: 2026-08-12

## 1. Feature Description

Admin-side (role >= 10) user management extensions built on top of the standard user CRUD:

- **Invitee history** — `GET /api/user/:id/invitees`: paginated view of **any** user's invited users (`inviter_id == :id`), returned as the minimal `dto.InviteeBrief` (**8 fields** — no `phone_region`, this repo has no such column).
- **User CSV export** — `POST /api/user/export`: CSV of **all** users, either by explicit `ids` or by `keyword` + `group` filter; batch 200, UTF-8 BOM, ASCII filename `users_<ts>.csv`; all batches are pre-fetched **before** anything is written, so a batch error fails the request instead of emitting a partial CSV. Requests are bounded (`maxExportIds = 10000` ids / `maxExportRows = 50000` rows, exceeded → `common.batch_too_many`).
- **User data dashboard** — `GET /api/user/:id/quota-dates`: admin proxy for a user's quota/usage trend (`quota_data` table), gated by `canViewUserDashboard`.

### Relationship with ops-role-system.md

> **本文档与 [ops-role-system.md](./ops-role-system.md) 是同一"邀请/用户管理"域下的两套互补文档**，分别记录不同角色的权限视图：
>
> - `ops-role-system.md` — **Ops 角色（role 5）** 视图：`/api/ops/user/*`。运营只能查看/搜索/导出**自己邀请**的用户（`inviter_id == self`），电话脱敏，只读，无数据看板。
> - 本文档 — **Admin 角色（role >= 10）** 视图：`/api/user/:id/invitees`、`/api/user/export`、`/api/user/:id/quota-dates`。管理员不受邀请范围限制，可查看任意用户的邀请记录、全量导出用户数据、代查用户数据看板。
>
> 两者的数据基础相同（`users.inviter_id` 邀请关系、`quota_data` 表），但**权限模型不同**：Ops 侧是"自证范围 + 脱敏"；Admin 侧是"全局可见"（导出/看板不受范围约束，看板另有角色等级校验 `canViewUserDashboard`）。
>
> **注意**：`controller/user_ops.go`（运营侧邀请管理）**已包含在** ops-role-system.md 第 2 节，本文档不重复记录；需要了解运营侧行为时请阅读该文档。

两套权限视图的具体对照：

| Capability | Ops (self-scoped, `OpsAuth`) | Admin (global, `AdminAuth`) |
|------------|------------------------------|------------------------------|
| Invitee list | `GET /api/ops/user/` (own invitees, phone masked, full User) | `GET /api/user/:id/invitees` (any user's invitees, unmasked phone, `InviteeBrief`) |
| CSV export | `POST /api/ops/user/export` (own invitees only, 12 cols, no phone column) | `POST /api/user/export` (all users, 14 cols incl. role/remark) |
| Data dashboard | — | `GET /api/user/:id/quota-dates` (role-hierarchy gated) |

### What an Admin can do

- View the invitee list of any user (including other admins' invitees) without phone masking — admins outrank the data owner.
- Export the full user table to CSV (selected ids, or keyword/group filtered), with role/status labels resolved for the request locale and `remark`, `aff_code`, `aff_count`, `inviter_id` columns.
- Query any user's daily quota/usage trend within a max 1-month window.

### What an Admin cannot do

- View the dashboard of a **higher** role: `canViewUserDashboard` requires `myRole > target.Role` (root 100 is the only exception). E.g. admin (10) cannot view root's (100) dashboard, while root (100) can view any user's dashboard — including another root's.
- Export/dashboard require `AdminAuth` (min role 10); ops (5) and common users are rejected.
- The invitees endpoint is read-only — no invite relationship mutation exists here (invitees are created at registration via `aff_code`).

---

## 2. Related Code and Code Logic

### Backend files

| File | Role |
|------|------|
| `controller/user_invitees.go` | `GetUserInvitees` — any-user invitee list (paged, `InviteeBrief`) |
| `controller/user_export.go` | `ExportUsers` + `formatUserRow` / `userExportHeaders` + `ExportUsersRequest` |
| `controller/user_quota_dates.go` | `GetUserQuotaDatesByAdmin` + `canViewUserDashboard` + `adminQuotaDatesMaxRange` |
| `dto/user_invitee.go` | `InviteeBrief` — minimal invitee list item (8 fields, no `phone_region`) |
| `dto/user_dashboard.go` | `UserDashboardBrief` + `UserDashboardPayload` — dashboard response DTOs |
| `model/user.go` | `GetUserInvitees` / `ExportUsersByIds` / `ExportUsersByFilter` — paged invitees + export batching (`userExportBatchSize = 200`) |
| `model/usedata.go` | `GetQuotaDataByUserId` — `quota_data` rows in `[start, end]` |
| `router/api-router.go` | Route registration under `AdminAuth` (adminRoute block) |
| `common/page_info.go` | `PageInfo` / `GetPageQuery` — shared pagination (page `p`, `page_size`) |

### Routes (`router/api-router.go`)

```text
adminRoute := userRoute.Group("/").Use(middleware.AdminAuth())   // userRoute: /api/user
  GET  /:id/invitees    GetUserInvitees          // 必须在 /:id 之前注册
  GET  /:id/quota-dates GetUserQuotaDatesByAdmin
  GET  /:id             GetUser                 // 通配兜底
  POST /export          ExportUsers
```

Note: `/:id/invitees` and `/:id/quota-dates` are registered **before** `GET /:id` (the router comment documents this as wildcard-conflict defense); there is a dedicated route-ordering test (`router/user_invitees_route_test.go` → `TestUserSubRouteOrder`).

### Auth flow (`middleware/auth.go`)

`AdminAuth` funnels through `authHelper(c, minRole)` (min = 10): session-based dashboard auth, or access-token fallback when there is no session, disabled-user rejection, then the role threshold. The three admin routes live on the `adminRoute` group, which carries `AdminAuth` directly — ops (5) and common (1) users, and unauthenticated requests, are all rejected at this layer.

### Invitee history (`controller/user_invitees.go` + `dto/user_invitee.go`)

- `GetUserInvitees` parses `:id` (invalid → `common.invalid_id`), builds `PageInfo` via `GetPageQuery`, and calls `model.GetUserInvitees(id, pageInfo)`.
- `model.GetUserInvitees` counts `inviter_id = ?`, then selects rows with `Omit("password", "access_token")` ordered `created_at DESC, id DESC`, offset/limited by the page info; the controller projects only `id, username, email, phone, status, group, role, created_at` into `dto.InviteeBrief`.
- Contrast with ops: the ops list (`GetOpsInvitees`) returns full `*User` rows with **masked** phone; the admin endpoint returns the minimal DTO with the **raw** phone (admin outranks the target).
- `InviteeBrief` (`dto/user_invitee.go`) has **8 fields** and is the only consumer of this query shape — deliberately no password/quota/token fields. (The searouter-isuanova source carried a `phone_region` field; this repo has no such column, so the brief stops at the raw `phone`.)

### CSV export (`controller/user_export.go`)

- Request: `ExportUsersRequest{ Ids []int, Keyword, Group, Format string }`; only `csv` (or empty) is accepted, else `admin.export_unsupported_format` (interpolating `{{.Format}}`).
- **Eager pre-fetch, fail before write**: `ExportUsers` fetches **all** batches via `model.ExportUsersByIds` / `model.ExportUsersByFilter` before writing anything; a batch error fails the request with `common.ApiError` — there is never a partial CSV. (Deliberately different from the ops export in `controller/user_ops.go`, which streams and stops gracefully; that behavior is not ported here.)
- Response: `Content-Type: text/csv; charset=utf-8` + `Content-Disposition: attachment; filename="users_<YYYYMMDD_HHMMSS>.csv"` (ASCII filename — non-ASCII `Content-Disposition` filenames render as garbage in browsers), prefixed with UTF-8 BOM for Excel.
- Header row `userExportHeaders(c)` (14 columns, order-stable for downstream scripts): `ID, 用户名, 显示名, 角色, 状态, 分组, 总额度, 已用额度, 请求次数, 创建时间, 备注, 邀请码, 邀请数, 邀请人 ID` — headers are i18n-resolved **per request locale** (`admin.export_header.*`), so e.g. a zh-CN request gets the Chinese header row.
- **By ids** (`model.ExportUsersByIds`): `WHERE id IN ?` batched at `userExportBatchSize = 200`, `Omit("password", "access_token")`; a batch error is logged (`ExportUsers ids batch err`) and returned.
- **By filter** (`model.ExportUsersByFilter`): empty keyword+group → `GetAllUsers(page)`; otherwise `SearchUsers(keyword, group, ...)` — pages of 200 until a short page; empty pages break the loop; a batch error is logged (`ExportUsers filter batch err`) and returned.
- `formatUserRow` resolves role/status labels for the request locale via `admin.role.*` / `admin.status.*` — including the Ops role 5 (`admin.role.ops`, which the source's `roleZh` did not know); unknown values fall back to `admin.role.unknown` / `admin.status.unknown` with the numeric value appended. `CreatedAt` is converted to `2006-01-02 15:04:05` (empty if 0); user-controlled cells pass through `csvSafeCell` to neutralize spreadsheet formula injection.
- Audit: every export logs `admin %d (%s) exported users count=%d mode=selected|filter` via `common.SysLog`.

### Data dashboard (`controller/user_quota_dates.go` + `dto/user_dashboard.go`)

- `canViewUserDashboard(myRole, targetRole)`: root (100) always allowed; otherwise `myRole > targetRole`. Admin can view ops (5)/common (1), not admin (10)/root (100); denied requests get `admin.cannot_view_user_dashboard`.
- Errors: invalid `:id` → `common.invalid_id`; nonexistent user → `user.not_exists`.
- Timestamps: `start_timestamp`/`end_timestamp` are parsed with `strconv.ParseInt`; parse failures, negative values, and `end < start` are rejected with `admin.quota_dates_range_exceeded` (with both non-negative and ordered, `end - start` cannot overflow int64).
- Range check: `end - start > adminQuotaDatesMaxRange (2592000 = 1 month)` → `admin.quota_dates_range_exceeded`; boundary `== 2592000` is allowed.
- Data: `model.GetQuotaDataByUserId(id, start, end)` (`model/usedata.go`) — raw `quota_data` rows.
- Response wraps `dto.UserDashboardPayload{ User: UserDashboardBrief{...}, Dates: any }` — the lightweight user brief avoids a second frontend fetch; `Dates` is `any` to avoid a model→dto→model import cycle.
- Frontend: `web/src/features/users/api.ts` `getUserQuotaDates` calls `GET /api/user/:id/quota-dates?start_timestamp=&end_timestamp=` and renders via `UserDashboardDialog`.

### Frontend

- `web/src/features/users/components/users-table.tsx` + `data-table-row-actions.tsx` — row actions open the `UserInviteesDialog` (invitees) and `UserDashboardDialog` (data dashboard).
- `web/src/features/users/components/dialogs/user-invitees-dialog.tsx` — paginated modal over `getUserInvitees` (`GET /api/user/:id/invitees`); columns: Username / Email / Phone / Status / Group / Role / Created At; raw phone only (no region — the brief has no `phone_region`); empty state `t('No invitees yet')` (zh: 暂无受邀用户).
- `web/src/features/users/components/user-dashboard-dialog.tsx` — data dashboard modal over `getUserQuotaDates`, rendering the quota/usage trend.
- `web/src/features/users/api.ts` — `getUserInvitees`, `getUserQuotaDates`, `exportUsers` (blob download, filename parsed from `Content-Disposition`).
- `web/src/features/users/lib/export-utils.ts` — `buildExportPayload(selectedIds, filter)`: selected ids win when present, otherwise the current table filter (keyword/group) is sent; an empty payload means "export all users" (mirrors the backend ids-over-filter rule).
- Export entry points: `web/src/features/users/components/users-primary-buttons.tsx` — 导出全部 (`Export All` → filter payload); `web/src/features/users/components/data-table-bulk-actions.tsx` — 导出选中 (`Export Selected` → ids).

---

## 3. Test Cases

**This repo's coverage** (the source doc's "Existing coverage" list, updated to what now exists):

### Backend

- `model/user_invitees_test.go` — `TestGetUserInviteesScopesAndOrders` (inviter scoping + ordering), `TestGetUserInviteesPagination`, `TestGetUserInviteesEmpty` (zero invitees → `total = 0`, empty items), `TestGetUserInviteesIgnoresTargetRole` (no role-based scoping at model level — scoping is purely `inviter_id`).
- `model/user_export_test.go` — `TestExportUsersByIds`, `TestExportUsersByFilterPaging` (200-batch pages until a short page), `TestExportUsersByFilterIncludesSoftDeleted`, `TestExportUsersByFilterBatchError` (batch error returned, not swallowed).
- `controller/user_invitees_test.go` — `TestGetUserInviteesReturnsBriefItems` (8 fields, no password/quota), `TestGetUserInviteesPagination` (`p`/`page_size`), `TestGetUserInviteesInvalidId` (0, non-numeric → `common.invalid_id`).
- `controller/user_export_test.go` — `TestFormatUserRow` / `TestFormatUserRowZeroCreatedAtAndOpsRole` (row formatting incl. empty `created_at` and the Ops role 5 label), `TestExportUsersWritesCsvByIds` (UTF-8 BOM + headers, every returned row corresponds to a requested id), `TestExportUsersWritesCsvByFilter` (keyword/group filter, empty = all), `TestExportUsersFailsWithoutWritingCsvOnDbError` (no partial CSV), `TestExportUsersCsvNeutralizesFormulas` (`csvSafeCell`), `TestExportUsersRejectsUnsupportedFormat` (`admin.export_unsupported_format`), `TestExportUsersRejectsMalformedJson` (`common.invalid_params`).
- `controller/user_quota_dates_test.go` — `TestCanViewUserDashboard` (role matrix), `TestGetUserQuotaDatesByAdminRoleMatrix` (admin sees common/ops, not admin/root; root sees anyone), `TestGetUserQuotaDatesByAdminRangeGuard` (cap and boundary), `TestGetUserQuotaDatesByAdminTimestampValidation` (malformed/negative/reversed/extreme timestamps rejected; valid extreme-but-ordered values pass), `TestGetUserQuotaDatesByAdminResponseShape` (`data.user` matches `UserDashboardBrief`, `data.dates` matches `GetQuotaDataByUserId` output), `TestGetUserQuotaDatesByAdminInvalidIdAndMissingUser`.
- `router/user_invitees_route_test.go` — `TestUserSubRouteOrder`: `/:id/invitees` registered before `/:id` (radix-tree ordering).

### Frontend (`bun test`)

- `web/src/features/users/lib/__tests__/export-utils.test.ts` — `buildExportPayload`: ids win over the filter; empty filter fields omitted; empty payload means "export all users".

**Adaptations from the source doc's test list**: the source's "Not covered" cases are all covered here. The suggested log-line `mode=selected|filter` assertions were dropped (tests assert CSV output, not the audit line); the suggested integration cases (ops rejected by `AdminAuth`; admin export of another admin's invitees succeeding) are covered at the **component level** — the auth middleware role-threshold tests cover ops-rejected-by-`AdminAuth`, and the model-level `TestGetUserInviteesIgnoresTargetRole` covers the absence of role scoping for invitees (no endpoint-level test exists: the controller tests bypass `AdminAuth` and the route test registers stub handlers); the suggested "batch errors log and stop gracefully" became `TestExportUsersFailsWithoutWritingCsvOnDbError` — the port deliberately fails the whole request instead of streaming a partial CSV; the suggested 9-field `InviteeBrief` checks became 8-field (no `phone_region`).

---

## 4. Known Limitations

- **Export is eager, not streamed** — all matching users are pre-fetched into memory before the CSV is written (the price of fail-before-write). Very large user tables consume memory proportional to the export size; requests are capped at `maxExportIds = 10000` ids / `maxExportRows = 50000` rows (`common.batch_too_many` when exceeded).
- **Dashboard range is capped at 1 month** (`adminQuotaDatesMaxRange`) per request; longer trends require multiple requests.
- **The admin invitees endpoint is a raw projection of the invite relationship** — scoped purely by `inviter_id`, with no additional role or group filtering.
