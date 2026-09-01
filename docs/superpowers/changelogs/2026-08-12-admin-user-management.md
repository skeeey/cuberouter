# Changelog

All notable changes to this project are documented here, newest first.

## Unreleased

### Added

#### Admin User Management (v1)

Admin-side (role >= 10) user management extensions ported from searouter-isuanova, built on top of the standard user CRUD: invitee history for **any** user, full-table CSV export, and an admin proxy for a user's quota/usage dashboard — all behind `AdminAuth`.

**Backend**

- `dto.InviteeBrief` (`dto/user_invitee.go`) — minimal 8-field invitee list item (id / username / email / phone / status / group / role / created_at; no `phone_region` — this repo has no such column); `UserDashboardBrief` + `UserDashboardPayload` (`dto/user_dashboard.go`).
- `model.GetUserInvitees` (`model/user.go`) — paged invitee query scoped `WHERE inviter_id = ?`, ordered `created_at DESC, id DESC`, `Omit("password", "access_token")`.
- `model.ExportUsersByIds` / `model.ExportUsersByFilter` (`model/user.go`) — export batching at `userExportBatchSize = 200` (ids via `id IN ?`, filter via `GetAllUsers` / `SearchUsers` paging until a short page); a batch error is logged (`ExportUsers ids batch err` / `ExportUsers filter batch err`) and returned so the caller fails the request.
- `controller.GetUserInvitees` (`controller/user_invitees.go`) — any-user invitee list with the raw phone (admins outrank the data owner; no phone masking); invalid `:id` → `common.invalid_id`.
- `controller.ExportUsers` (`controller/user_export.go`) — **eager pre-fetch of all batches before writing**, so a batch error fails the request instead of emitting a partial CSV (the source's stream-and-stop-gracefully behavior is deliberately not ported); requests are bounded at `maxExportIds = 10000` ids / `maxExportRows = 50000` rows (`common.batch_too_many` when exceeded); ASCII filename `users_<ts>.csv`; UTF-8 BOM for Excel; 14 column headers resolved per request locale (`admin.export_header.*`); role/status labels via `admin.role.*` / `admin.status.*` (including the Ops role 5, which the source's `roleZh` did not know); `csvSafeCell` neutralizes spreadsheet formula injection; audit log `admin %d (%s) exported users count=%d mode=selected|filter`.
- `controller.GetUserQuotaDatesByAdmin` (`controller/user_quota_dates.go`) — dashboard proxy gated by `canViewUserDashboard` (`myRole > targetRole`, root 100 exception; denied → `admin.cannot_view_user_dashboard`) with a 1-month range cap (`adminQuotaDatesMaxRange = 2592000`; exceeded → `admin.quota_dates_range_exceeded`); timestamps are validated (parse failures, negative values, and reversed intervals are rejected) so extreme values cannot overflow the range check; errors `common.invalid_id` / `user.not_exists`.
- Routes under the `AdminAuth` adminRoute block in `router/api-router.go`: `GET /:id/invitees`, `GET /:id/quota-dates`, `POST /export` — specific paths registered before the `/:id` wildcard, with a dedicated route-order test.
- Backend i18n keys `admin.*` in en, zh-CN, and zh-TW locales (`i18n/locales/*.yaml`).

**Frontend**

- `web/src/features/users/api.ts` — `getUserInvitees`, `getUserQuotaDates`, `exportUsers` (blob download, filename parsed from `Content-Disposition`).
- `web/src/features/users/lib/export-utils.ts` — `buildExportPayload`: selected ids win over the current table filter; empty fields omitted so an empty payload means "export all users".
- `web/src/features/users/components/dialogs/user-invitees-dialog.tsx` — paginated invitee history modal (raw phone only, empty state).
- `web/src/features/users/components/user-dashboard-dialog.tsx` — quota/usage trend dialog over `getUserQuotaDates`.
- Export entry points: `users-primary-buttons.tsx` (导出全部 / Export All), `data-table-bulk-actions.tsx` (导出选中 / Export Selected); row actions in `data-table-row-actions.tsx` open the invitees / dashboard dialogs.
- i18n strings in en, zh, and zh-TW locales.

**Docs**

- Design spec: `docs/superpowers/specs/2026-08-12-admin-user-management-design.md`; operator guide: `docs/admin-user-management.md`.

**Tests**

- `model/user_invitees_test.go` — inviter scoping/ordering, pagination, empty set, target-role independence.
- `model/user_export_test.go` — ids batching, filter paging (200-batch), soft-deleted inclusion, batch-error propagation.
- `controller/user_invitees_test.go` — 8-field brief projection, pagination, invalid id.
- `controller/user_export_test.go` — row formatting (incl. the Ops role label and zero `created_at`), BOM + headers by ids/filter, fail-without-writing on DB error, formula neutralization, unsupported-format and malformed-JSON rejection.
- `controller/user_quota_dates_test.go` — `canViewUserDashboard` role matrix, range guard, response shape, invalid id / missing user.
- `router/user_invitees_route_test.go` — `/:id/invitees` registered before `/:id`.
- Frontend: `web/src/features/users/lib/__tests__/export-utils.test.ts` (`buildExportPayload`).

**Known limitations**

- Export is eager, not streamed — all matching users are pre-fetched into memory before the CSV is written, so memory scales with the export size (capped at `maxExportIds = 10000` ids / `maxExportRows = 50000` rows → `common.batch_too_many`).
- Dashboard range capped at 1 month (`adminQuotaDatesMaxRange = 2592000`) per request; longer trends require multiple requests.
