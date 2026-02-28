# FINDINGS

Technical discoveries and gotchas encountered during development.

---

## 2026-02-28

### Databricks SCIM: group-type members have empty `display` and unreliable `$type`/`$ref`

**Affected code**: `internal/tui/screens/identity.go` — list view group member rendering

**Finding**: The Databricks SCIM API (`GET /api/2.0/preview/scim/v2/Groups`) returns members
where:
- `display` is populated for Users/SPs but **often empty for Group-type members**
- `$type` may be absent and `$ref` may not match expected patterns (`/Groups/`, `/Users/`, etc.)
  causing the type to fall back to `"Unknown"` for **any** member kind (not just groups)

Both fields are reliable only for the member `value` (SCIM ID).

**Impact**: In the list view, expanding a group that contained subgroups showed nothing or
showed gray `[unknown]` entries with no name. In the tree view, subgroups were not identified
as children (so they appeared as root groups) and their members (users with type "Unknown")
were silently dropped from the rendered tree.

**Fix**: Always use ID-based lookup rather than `m.Type` for type resolution:
- List view: build `groupNameByID` map; override type to `"Group"` when member ID is in the map.
- Tree view (`buildGroupTreeItems`): use `groupByID[m.ID]` existence check for `childGroupIDs`
  and `inGroup` instead of checking `m.Type`.
- Tree view (`renderGroupTreeNode`): resolve `resolvedType` via `groupByID` / `userByID` /
  `spByID` ID lookup before the switch; fall back to `m.DisplayName` enrichment from the
  canonical user/SP slices when `m.DisplayName` is empty.
- `groupMatchesDeep`: use `groupByID[m.ID]` existence check instead of `m.Type == "Group"`.

**Reference**: `buildWorkspaceItems()` in [internal/tui/screens/identity.go](internal/tui/screens/identity.go)

---

### Databricks SDK: `NewWorkspaceClient` does not validate credentials at construction time

**Finding**: `dbsdk.NewWorkspaceClient(cfg)` succeeds even with invalid or expired credentials.
Auth errors only surface on the first actual API call.

**Impact**: The original startup flow silently accepted broken auth configs; errors only
appeared inside the TUI after it started.

**Fix**: Added `factory.ValidateAuth(ctx, ws)` in `internal/auth/startup.go` which calls
`client.CurrentUser.Me(ctx)` as a lightweight pre-TUI auth probe.

---

### Go module path was placeholder `github.com/you/`

**Finding**: The project was initialized with `module github.com/you/dbx-dash` in `go.mod`.
All internal imports used this placeholder path.

**Fix**: Replaced all occurrences with `github.com/tttao/dbx-dash` across `go.mod`, all
`.go` files, and `README.md`.
