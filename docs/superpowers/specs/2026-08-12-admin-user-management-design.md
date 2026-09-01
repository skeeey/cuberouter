# Admin User Management Extensions Port — Design Spec

> Date: 2026-08-12
> Source: searouter-isuanova (private source repo) — `docs/admin-user-management.md`
> Approach: **Faithful adapted port** — mirror the source backend structure 1:1, adapted to this repo's conventions (i18n keys, `csvSafeCell`, ASCII filenames, model-layer batching, no `phone_region` column); rewrite the frontend in this repo's TanStack Router + TypeScript + Base UI + VChart stack; port the source's existing tests and add the ones it lacks.

## 1. Scope decisions (approved by user)

| Decision | Choice |
|---|---|
| Backend | **Full port**: `InviteeBrief` + dashboard DTOs, `GetUserInvitees` model fn, two export model fns, three controllers (`user_invitees.go`, `user_export.go`, `user_quota_dates.go`), routes under the existing admin user group, i18n keys |
| `phone_region` in `InviteeBrief` | **Dropped** — this repo's `User` model has no phone-region column anywhere (campaign port added `Phone` only; ops invitee export also omits it). `InviteeBrief` becomes 8 fields; the invitees dialog shows the raw phone only. No schema change. |
| Export behavior | **Ops-port upgraded pattern (A1)**: i18n-resolved headers per request locale, `csvSafeCell` formula-injection guard, ASCII filename `users_<ts>.csv`, batching in the model layer, and **pre-fetch all batches before writing the CSV header** so a batch DB error fails the request instead of emitting a truncated CSV that looks successful. The source's streaming-with-graceful-stop behavior (and its test case 4) is deliberately not ported. |
| Frontend | **Full port, rewritten** in this repo's stack inside `web/src/features/users/`: invitees dialog, compact VChart dashboard modal (7d default window, preset selector), 导出全部 / 导出选中 export buttons. No new routes or sidebar entries. |
| Dashboard rendering | **Compact VChart modal** (user-approved): reuse `lib/charts.ts` VChart primitives + `formatChartTime`/currency helpers; stat row from `UserDashboardBrief`. No changes to the existing `features/dashboard` page. |
| Tests | Port the source's existing tests (`model/user_invitees_test.go`, `controller/user_export_test.go`, `router/user_invitees_route_test.go`) and add the source doc's not-covered cases (invitees/export/quota-dates controllers, `canViewUserDashboard`, DTO shapes). |
| Log-line `mode=` assertions (source doc cases 1–3) | **Dropped** — no SysLog capture harness exists; the ops export tests set the precedent of asserting response bytes only. |
| Integration cases 10–11 | **No new tests** — ops-rejected-by-`AdminAuth` is already covered by the existing `authHelper` minRole tests; admin-export-has-no-inviter-scoping is implied by the export model tests (no `inviter_id` filter). Noted in the ported doc. |
| Error messages | i18n keys (`en`/`zh-CN`/`zh-TW`), following the ops-port key style; reuse existing `common.invalid_id`, `user.not_exists`, `common.invalid_params` where the message matches |
| Role labels in export | Includes the ops role (5) — this repo has `RoleOpsUser`, which the source's `roleZh` did not know |

## 2. Feature summary

Admin-side (role >= 10) user management extensions built on the standard user CRUD:

- **Invitee history** — `GET /api/user/:id/invitees`: paginated view of **any** user's invited users (`inviter_id == :id`), returned as the minimal `InviteeBrief` (8 fields, raw phone — admins outrank the data owner, no masking).
- **User CSV export** — `POST /api/user/export`: eagerly prefetched CSV of **all** users, either by explicit `ids` or by `keyword` + `group` filter; batch 200; UTF-8 BOM; i18n headers per request locale; 14 columns including role/status labels and `remark`, `aff_code`, `aff_count`, `inviter_id`; bounded at `maxExportIds = 10000` ids / `maxExportRows = 50000` rows (exceeded → `common.batch_too_many`).
- **User data dashboard** — `GET /api/user/:id/quota-dates`: admin proxy for a user's quota/usage trend (`quota_data` table), gated by `canViewUserDashboard` (`myRole > targetRole`, root exempt) and a max 1-month window.

Both `/api/user/:id/invitees` and `/api/user/export` and `/api/user/:id/quota-dates` sit under `AdminAuth` (min role 10); ops (5) and common users are rejected. The invitees endpoint is read-only — invite relationships are created at registration via `aff_code`.

## 3. Backend design

### 3.1 `dto/user_invitee.go` (new)

`InviteeBrief` — 8 fields (source's `phone_region` dropped per §1): `Id, Username, Email, Phone, Status, Group, Role, CreatedAt`. JSON tags as in the source (`id`, `username`, `email`, `phone`, `status`, `group`, `role`, `created_at`).

### 3.2 `dto/user_dashboard.go` (new)

Verbatim port:

- `UserDashboardBrief` — `Id, Username, DisplayName, Role, Group, Quota, UsedQuota, RequestCount`.
- `UserDashboardPayload{ User UserDashboardBrief; Dates any }` — `Dates any` keeps the `any` to avoid a `model→dto→model` import cycle (as in the source).

### 3.3 `model/user.go` (extend)

- **`GetUserInvitees(inviterId int, pageInfo *common.PageInfo) ([]*User, int64, error)`** — repo-idiom deviation from the source: the source scans a raw `SELECT ... AS "group"` projection into `[]dto.InviteeBrief` (reserved-word alias, model→dto import). This port follows the ported `GetOpsInvitees` shape instead: count `inviter_id = ?`, then `Omit("password", "access_token")`, `Order("created_at DESC, id DESC")`, offset/limit. The controller projects into `InviteeBrief` — identical wire shape, no raw SQL, no import cycle.
- **`ExportUsersByIds(ids []int) ([]*User, error)`** — batches at `userExportBatchSize = 200` (`id IN ?` per batch), `Omit("password", "access_token")`; a batch error is logged via `common.SysLog` (`ExportUsers ids batch err: %v`) and **returned** so the caller fails the request (A1 behavior — the ops port's `ExportOpsInviteesByIds` precedent).
- **`ExportUsersByFilter(keyword, group string, maxRows int) ([]*User, error)`** — pages of 200: empty keyword+group → `GetAllUsers`; otherwise `SearchUsers(keyword, group, ...)`. Stops on a short page or `len == 0`; batch error logged (`ExportUsers filter batch err: %v`) and returned. `maxRows > 0` bounds the export: when the first page's `total` exceeds it, `ErrExportRowsExceeded` is returned before further rows are fetched (the controller passes `maxExportRows = 50000`). Note: this repo's `GetAllUsers`/`SearchUsers` are `Unscoped()` (include soft-deleted users) — the export matches the users table the export button lives on.

### 3.4 `controller/user_invitees.go` (new)

`GetUserInvitees` — parse `:id` (`strconv.Atoi`, `<= 0` → `common.ApiErrorI18n(c, i18n.MsgInvalidId)`); `common.GetPageQuery(c)`; `model.GetUserInvitees`; project `[]*User → []dto.InviteeBrief` (exactly the 8 DTO fields — no password/quota/token over the wire); `pageInfo.SetTotal/SetItems`; `common.ApiSuccess`. Read-only, no mutation.

### 3.5 `controller/user_export.go` (new)

- `userExportHeaders(c *gin.Context) []string` — 14 headers resolved per request locale via new `admin.export_header.*` keys: ID, 用户名, 显示名, 角色, 状态, 分组, 总额度, 已用额度, 请求次数, 创建时间, 备注, 邀请码, 邀请数, 邀请人 ID (order-stable for downstream scripts).
- `formatUserRow(c, u) []string` — mirrors `formatOpsUserRow`: id, username, display_name, role label, status label, group, quota, used_quota, request_count, created_at (`2006-01-02 15:04:05`, empty if 0), remark, aff_code, aff_count, inviter_id. `csvSafeCell` on username/display_name/remark/aff_code/group. Role label via new `admin.role.*` keys (common/ops/admin/root + `Unknown(%d)` fallback); status label via new `admin.status.*` keys (enabled/disabled/unknown).
- `ExportUsers` — `ExportUsersRequest{Ids []int, Keyword, Group, Format string}`; `common.DecodeJson` failure → `i18n.MsgInvalidParams`; non-`csv` format → `admin.export_unsupported_format` with `{{.Format}}`; `len(Ids) > maxExportIds (10000)` → `common.batch_too_many` with `Max`, and the filter path passes `maxExportRows (50000)` into `ExportUsersByFilter` (`ErrExportRowsExceeded` → the same `common.batch_too_many`). Pre-fetch via `ExportUsersByIds` (ids non-empty) or `ExportUsersByFilter` (else) **before** writing anything; error → `common.ApiError` (no partial file). Then: ASCII filename `users_<ts>.csv`, `Content-Type: text/csv; charset=utf-8`, `Content-Disposition attachment`, UTF-8 BOM, header row, row stream + flush. Audit via `common.SysLog`: `admin %d (%s) exported users count=%d mode=selected|filter`.

### 3.6 `controller/user_quota_dates.go` (new)

- `adminQuotaDatesMaxRange = 2592000` (1 month in seconds).
- `canViewUserDashboard(myRole, targetRole int) bool` — root always `true`; otherwise `myRole > targetRole`. Admin (10) can view ops (5)/common (1), not admin (10)/root (100); root (100) can view anyone.
- `GetUserQuotaDatesByAdmin` — parse `:id` → `MsgInvalidId`; `model.GetUserById(id, false)` error → `MsgUserNotExists`; `canViewUserDashboard(c.GetInt("role"), target.Role)` fail → `admin.cannot_view_user_dashboard`; `start_timestamp`/`end_timestamp` are parsed with `strconv.ParseInt` and rejected with `admin.quota_dates_range_exceeded` on parse failure, negative values, or `end < start` (with both non-negative and ordered, `end - start` cannot overflow int64); `end - start > adminQuotaDatesMaxRange` → the same error; then `model.GetQuotaDataByUserId(id, start, end)` → `common.ApiSuccess` with `dto.UserDashboardPayload{User: UserDashboardBrief{...}, Dates: dates}`.

### 3.7 `router/api-router.go`

Extend the existing `adminRoute` block (line ~132) — the two `/:id/*` subpaths registered **above** `GET /:id` (precedent: `/:id/oauth/bindings` already sits above it; the source has a dedicated route-ordering test, ported in §5):

```text
GET  /:id/invitees     GetUserInvitees           // before /:id
GET  /:id/quota-dates  GetUserQuotaDatesByAdmin  // before /:id
POST /export           ExportUsers               // no static/param conflict (no POST /:id exists)
```

### 3.8 Backend i18n (`i18n/keys.go` + `locales/{en,zh-CN,zh-TW}.yaml`)

New keys (ops-port style, `noun.verb` + `{{.X}}` args):

- `admin.cannot_view_user_dashboard` — "无权查看该用户的数据看板"
- `admin.quota_dates_range_exceeded` — "时间跨度不能超过 1 个月" (matches the wording already hardcoded in the self-service `GetUserQuotaDates`)
- `admin.export_unsupported_format` — `unsupported format: {{.Format}}`
- `admin.export_header.id|username|display_name|role|status|group|quota|used_quota|request_count|created_at|remark|aff_code|aff_count|inviter_id` (×14)
- `admin.role.common|ops|admin|root|unknown` (unknown takes `{{.Role}}`)
- `admin.status.enabled|disabled|unknown`

Reused: `common.invalid_id`, `user.not_exists`, `common.invalid_params`.

## 4. Frontend design

All inside the existing `web/src/features/users/` module. No new routes or sidebar entries. All user-facing strings via `t()` into `en.json`/`zh.json`/`zh-TW.json`.

### 4.1 `web/src/features/users/api.ts` (extend)

- `getUserInvitees(userId, params)` → `GET /api/user/:id/invitees?p=&page_size=` (typed envelope, zod schema).
- `getUserQuotaDates(userId, params)` → `GET /api/user/:id/quota-dates?start_timestamp=&end_timestamp=`.
- `exportUsers(payload)` → `POST /api/user/export` with `{ids} | {keyword, group}`; `text/csv` blob download, same pattern as the ops export.

### 4.2 `web/src/features/users/types.ts` (extend)

`InviteeBrief` (8 fields), `UserDashboardBrief`, `QuotaDataItem` (maps the backend `quota_data` row: `model_name`, `created_at`, `quota`, `count`, `token_used`), response envelopes.

### 4.3 `components/dialogs/user-invitees-dialog.tsx` (new)

Opened from a per-row 邀请记录 action in `data-table-row-actions.tsx`. Paginated dialog table: username / email / phone (raw — no masking) / status / group / role / created_at; loads page 1 on open, resets on close, empty state 暂无受邀用户.

### 4.4 `components/user-dashboard-dialog.tsx` (new)

Opened from a per-row 数据看板 action. Compact VChart modal (user-approved):

- Fetches `quota-dates` on open; 7-day default window with a preset selector (7d / 14d / 1 month — the backend 2592000s cap).
- Quota over time via the existing dashboard chart lib (`lib/charts.ts` VChart primitives, `formatChartTime`, currency helpers): bars for quota, line for usage/tokens; stat row from `UserDashboardBrief` (quota, used_quota, request_count).
- Backend errors (e.g. 无权查看该用户的数据看板) surface as toasts.

### 4.5 Export buttons

- `users-primary-buttons.tsx` — 导出全部 → `exportUsers({keyword, group})` with the current table filter state.
- `data-table-bulk-actions.tsx` — 导出选中 → `exportUsers({ids})` when rows are selected.
- Both use the existing blob-download + success/error toast patterns.

### 4.6 Frontend i18n

`en.json`/`zh.json`/`zh-TW.json`: dialog titles, invitee column labels, 导出选中/导出全部, empty states, error/success toasts.

## 5. Testing

Repo style: sqlite in-memory fixtures, `require` for setup/fatal, `assert` for value checks, deterministic tables.

### 5.1 `model/user_invitees_test.go` (new)

1. `GetUserInvitees` — only `inviter_id == caller`; no role-based scoping at model level (target role irrelevant — scoping is purely `inviter_id`); order `created_at DESC, id DESC`; pagination (`page_size`/offset); `password`/`access_token` never present in returned rows; zero invitees → `total = 0`, empty items (source doc case 9).

### 5.2 `model/user_export_test.go` (new)

2. `ExportUsersByIds` — respects all requested ids, batches at 200, no password/access_token (source doc case 4, model level).
3. `ExportUsersByFilter` — pages until a short page; empty keyword+group → `GetAllUsers` path; keyword+group → `SearchUsers` path.

### 5.3 `controller/user_invitees_test.go` (new)

4. `GetUserInvitees` — response items are 8-field `InviteeBrief` (no password/quota fields in JSON); `p`/`page_size` respected; invalid `:id` (0, non-numeric) → `common.invalid_id` message (source doc case 5, adapted — no phone_region).

### 5.4 `controller/user_export_test.go` (new)

5. `ExportUsers` with `ids` — UTF-8 BOM + 14 i18n headers; every returned row corresponds to a requested id; ids take precedence over keyword (source doc case 1, log-line assertion dropped).
6. `ExportUsers` with keyword/group (and empty) — filter results eagerly prefetched before the CSV is written (source doc case 2, log-line dropped).
7. `ExportUsers` rejects `format: "xlsx"` with `admin.export_unsupported_format`; rejects malformed JSON body with `common.invalid_params` (source doc case 3).
8. Batch DB error → **no CSV bytes written** (A1 behavior; mirrors `TestExportOpsInviteesFailsWithoutWritingCsvOnDbError`); `csvSafeCell` neutralizes `=...` formula cells; rows omit password (source doc case 4, adapted).

### 5.5 `controller/user_quota_dates_test.go` (new)

9. `GetUserQuotaDatesByAdmin` — admin (10) views common (1) and ops (5); admin rejected for admin (10) and root (100) with `admin.cannot_view_user_dashboard`; root (100) views anyone (source doc case 6).
10. Range guard — `end - start > 2592000` → `admin.quota_dates_range_exceeded`; boundary `== 2592000` allowed (source doc case 7).
11. Timestamp validation — malformed, negative, reversed, and extreme int64 timestamps all rejected with `admin.quota_dates_range_exceeded`; valid extreme-but-ordered values still pass (table-driven `TestGetUserQuotaDatesByAdminTimestampValidation`).
12. Response shape — `data.user` matches `UserDashboardBrief` fields; `data.dates` matches `GetQuotaDataByUserId` output (source doc case 8).

### 5.6 `router/user_invitees_route_test.go` (new)

13. `/:id/invitees` and `/:id/quota-dates` registered before `GET /:id` (radix-tree ordering) — port of the source's route-ordering test.

### 5.7 Frontend (adapted source doc cases 12–13)

14. `buildExportPayload` (`web/src/features/users/lib/__tests__/export-utils.test.ts`) — ids win over the filter; empty filter fields omitted; empty payload means "export all users". The delivered frontend test covers `buildExportPayload` only — `getUserInvitees`/`getUserQuotaDates` query construction and the `exportUsers` blob behavior have no dedicated Vitest coverage.

## 6. Docs

- Changelog: `docs/superpowers/changelogs/2026-08-12-admin-user-management.md` (Added section, mirroring the ops changelog structure).
- Operator guide: `docs/admin-user-management.md` (adapted from the source doc: `phone_region` dropped, i18n keys instead of hardcoded strings, ASCII filename, fail-before-writing export behavior, re-pointed file:line refs, cross-reference to `docs/ops-role-system.md` kept, test section updated to reflect the tests that now exist).
