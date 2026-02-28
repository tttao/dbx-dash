# dbx-dash: Technical Architecture

## Overview

`dbx-dash` is a terminal dashboard for Databricks workspaces, written in Go.
It provides live visibility into jobs, clusters, SQL warehouses, DLT pipelines,
and workspace identity (users, groups, service principals) across multiple workspaces.

## Tech Stack

| Layer | Library |
|---|---|
| TUI framework | `github.com/charmbracelet/bubbletea` v1.2.4 |
| Styling | `github.com/charmbracelet/lipgloss` v1.0.0 |
| Widgets | `github.com/charmbracelet/bubbles` v0.20.0 |
| Databricks API | `github.com/databricks/databricks-sdk-go` v0.55.0 |
| CLI | `github.com/spf13/cobra` |
| Config (INI) | `gopkg.in/ini.v1` |
| Config (TOML) | `github.com/BurntSushi/toml` |
| Cache (SQLite) | `modernc.org/sqlite` (pure Go, no CGO) |

## Project Structure

```
dbx-dash/
├── cmd/
│   └── dbx-dash/
│       └── main.go                   # cobra CLI entrypoint
├── internal/
│   ├── config/
│   │   ├── model.go                  # WorkspaceProfile, AppConfig, RefreshConfig
│   │   └── loader.go                 # merge ~/.databrickscfg (INI) + ~/.dbx-dash/config.toml
│   ├── databricks/
│   │   ├── interfaces.go             # provider interfaces (JobsProvider, etc.)
│   │   ├── models.go                 # domain types (Job, Cluster, UserDetail, etc.)
│   │   ├── sdk/
│   │   │   ├── factory.go            # WorkspaceClientFactory (lazy init, per-profile cache)
│   │   │   ├── jobs.go               # SDKJobsProvider
│   │   │   ├── clusters.go           # SDKClustersProvider
│   │   │   ├── warehouses.go         # SDKWarehousesProvider
│   │   │   ├── pipelines.go          # SDKPipelinesProvider
│   │   │   └── identity.go           # SDKIdentityProvider
│   │   └── mock/
│   │       └── providers.go          # in-memory mocks for unit tests
│   ├── cache/
│   │   ├── db.go                     # SQLite init, schema versioning
│   │   └── repository.go             # UpsertJob, GetJobs, UpsertCluster, GetClusters
│   ├── tui/
│   │   ├── app.go                    # root Bubble Tea model (Update/View/Init)
│   │   ├── keys.go                   # global key bindings
│   │   ├── styles.go                 # shared lipgloss styles
│   │   └── screens/
│   │       ├── dashboard.go          # workspace health summary cards
│   │       ├── jobs.go               # sortable jobs + recent runs table
│   │       ├── clusters.go           # clusters table
│   │       ├── warehouses.go         # SQL warehouses table
│   │       ├── run_detail.go         # job run detail + log viewport
│   │       └── identity.go           # identity tree + detail popup
│   └── alerts/
│       └── terminal.go               # RingBell(), FlashAlert()
├── docs/
│   └── architecture.md               # this file
├── go.mod
└── README.md
```

## Data Layer: Provider Interfaces

All Databricks API calls go through provider interfaces defined in
`internal/databricks/interfaces.go`. This allows the SDK implementation to be
swapped for mocks in tests, or a future REST implementation.

```go
type JobsProvider interface {
    ListJobs(ctx context.Context, limit int) ([]Job, error)
    GetJob(ctx context.Context, jobID int64) (*Job, error)
    ListRecentRuns(ctx context.Context, jobID int64, limit int) ([]JobRun, error)
    GetRunOutput(ctx context.Context, runID int64) (*RunOutput, error)
}

type ClustersProvider interface {
    ListClusters(ctx context.Context) ([]Cluster, error)
    GetCluster(ctx context.Context, clusterID string) (*Cluster, error)
}

type WarehousesProvider interface {
    ListWarehouses(ctx context.Context) ([]SqlWarehouse, error)
    GetWarehouse(ctx context.Context, warehouseID string) (*SqlWarehouse, error)
}

type PipelinesProvider interface {
    ListPipelines(ctx context.Context) ([]Pipeline, error)
    GetPipeline(ctx context.Context, pipelineID string) (*Pipeline, error)
    ListPipelineUpdates(ctx context.Context, pipelineID string, limit int) ([]PipelineUpdate, error)
}

type IdentityProvider interface {
    ListGroups(ctx context.Context) ([]Group, error)
    ListUsers(ctx context.Context) ([]IdentityUser, error)
    ListServicePrincipals(ctx context.Context) ([]WorkspaceServicePrincipal, error)
    GetUser(ctx context.Context, userID string) (*UserDetail, error)
    GetServicePrincipal(ctx context.Context, spID string) (*SPDetail, error)
}
```

All five providers are bundled per workspace in `WorkspaceProviders`.

## Domain Models

Key types in `internal/databricks/models.go`:

| Type | Description |
|---|---|
| `Job` | Databricks job definition |
| `JobRun` | Single job execution with state/result |
| `RunOutput` | Logs and error output for a completed run |
| `Cluster` | All-purpose or job cluster |
| `SqlWarehouse` | SQL warehouse (serverless or classic) |
| `Pipeline` | Delta Live Tables pipeline |
| `PipelineUpdate` | Single DLT pipeline update run |
| `Group` | Workspace SCIM group with members |
| `GroupMember` | Member of a group (user, group, or SP) |
| `IdentityUser` | Workspace SCIM user (summary) |
| `WorkspaceServicePrincipal` | Workspace service principal (summary) |
| `UserDetail` | Full SCIM user including entitlements, groups, roles |
| `SPDetail` | Full SCIM service principal including entitlements, groups, roles |

### UserDetail / SPDetail

Used for the identity detail popup. Key fields:

```
Entitlements        []string             // "workspace-access", "allow-cluster-create", "databricks-sql-access"
Groups              []string             // direct SCIM group memberships (display names); fallback only
Roles               []string             // "admin", "user"
CatalogPermissions  []CatalogPermission  // nil = UC unavailable; empty = no grants
```

`CatalogPermission` holds catalog-level Unity Catalog grants:
```
CatalogName string
Privileges  []string  // e.g. "USE_CATALOG", "SELECT", "MODIFY"
```

## SDK Implementation

`internal/databricks/sdk/` wraps `databricks-sdk-go`.

**SDK limits (enforced in code):**
- `ListJobs`: max page size 100
- `ListRuns`: max page size 24

`WorkspaceClientFactory` creates one `*databricks.WorkspaceClient` per profile name,
lazily on first use, protected by a `sync.Mutex`. Auth: PAT token or OAuth M2M via
`databricks.Config{Host, Token, ClientID, ClientSecret}`.

The identity provider uses workspace-level SCIM APIs and Unity Catalog grants:
- `w.Groups.ListAll` with `Attributes:"id,displayName,members"` (list view)
- `w.Users.ListAll` with `Attributes:"id,userName,displayName,active"` (list view)
- `w.ServicePrincipals.ListAll` with `Attributes:"id,applicationId,displayName,active"` (list view)
- `w.Users.Get` (full SCIM detail: entitlements, groups, roles)
- `w.ServicePrincipals.Get` (full SCIM detail)
- `w.Catalogs.ListAll` + `w.Grants.GetEffective` per catalog (Unity Catalog permissions)
  - `GetEffective` is called with `Principal = user.UserName` (for users) or `Principal = sp.ApplicationID` (for SPs)
  - Returns `nil` (not an error) when Unity Catalog is unavailable

## TUI Architecture

The app uses the Elm-like Bubble Tea pattern: a root `Model` holds all screen
sub-models and routes messages.

### Root Model (`internal/tui/app.go`)

```
Model
├── screen     screenID      (current active screen)
├── wsIdx      int           (current workspace index)
├── providers  map[string]*WorkspaceProviders
├── dashboard  DashboardModel
├── jobs       JobsModel
├── clusters   ClustersModel
├── warehouses WarehousesModel
├── identity   IdentityModel
└── runDetail  RunDetailModel
```

### Async Data Loading

Data is fetched asynchronously via `tea.Cmd`. Each screen defines its own
load command; the root model fans them out across workspaces via `tea.Batch`.

Example flow for identity detail popup:
1. User presses Enter on a user row.
2. `IdentityModel.Update` returns `LoadUserDetailRequestMsg`.
3. `app.go` catches it, fires `screens.LoadUserDetailCmd(ctx, userID, provider)`.
4. `UserDetailLoadedMsg` is routed back to `IdentityModel.Update`.
5. Identity screen renders the popup overlay.

### Key Bindings (global)

| Key | Action |
|---|---|
| `1` | Dashboard |
| `2` | Jobs |
| `3` | Clusters |
| `4` | Warehouses |
| `5` | Identity |
| `r` | Refresh current screen |
| `w` | Next workspace |
| `enter` | Select / drill-down |
| `esc` / `b` | Back |
| `q` | Quit |

Global keys are suppressed when the identity search box is focused or the
detail popup is open.

### Identity Screen

The identity screen renders a per-workspace tree:

```
▼ production-workspace
    GROUPS (12)
    ▼ admins (3)
       ├ alice@example.com   [user]
       ├ bob@example.com     [user]
       └ etl-bot             [sp]
    ► data-engineers (7)
    USERS (45)
       alice@example.com
       bob@example.com  <bob.smith@example.com>
    SERVICE PRINCIPALS (3)
       etl-bot  app:1234567890
▼ dev-workspace
    ...
```

- `/` opens search filter (narrows groups by name/member, users by username/displayName,
  SPs by name/applicationID)
- `enter` on a workspace or group: expand/collapse
- `enter` on a user or SP: open detail popup

**Detail popup** (triggered by pressing `enter` on a user or SP row) shows:

| Section | Content |
|---|---|
| Header | Username/name, SCIM ID, active status |
| WORKSPACE ENTITLEMENTS | Direct entitlements on the workspace |
| ROLES | Workspace roles (admin/user) |
| GROUP MEMBERSHIPS | Direct groups (●) and transitive/inherited groups (◌) |
| UNITY CATALOG PERMISSIONS | Per-catalog privileges; "not available" if UC is disabled |

**Transitive group membership** is computed client-side from the already-loaded workspace
groups via BFS: starting from the entity's SCIM ID, the algorithm traverses the reverse
membership map (memberID -> groupIDs) until no new groups are found. Direct and indirect
groups are shown separately. No additional API calls are needed.

**Catalog permissions** are fetched by listing all catalogs (`Catalogs.ListAll`) and
querying `Grants.GetEffective` per catalog with `Principal` filter. If Unity Catalog
is not enabled or the caller lacks access, the section shows "(Unity Catalog not available)"
without surfacing an error to the user.

Dismiss popup with `esc` or `enter`.

## Config

Configuration is loaded from two sources merged at startup:

1. `~/.databrickscfg` (INI): workspace profiles with auth credentials
2. `~/.dbx-dash/config.toml` (optional overlay): display names, refresh intervals

```toml
[refresh]
default_interval = 30

[workspace.production]
display_name = "Prod"
refresh_interval = 60
```

Auth types supported: PAT (`token =`), OAuth M2M (`client_id` + `client_secret`).

## Cache

SQLite via `modernc.org/sqlite` (pure Go, no CGO, portable across platforms).
Schema versioned via `PRAGMA user_version`. Tables: `job_snapshots`, `cluster_snapshots`.

## Build

```bash
go build -o dbx-dash ./cmd/dbx-dash
./dbx-dash run
./dbx-dash config list
./dbx-dash config check <workspace>
```

The binary is self-contained. No CGO required.

## Cross-platform

The build is **not platform-specific** by default. `modernc.org/sqlite` is pure Go
(no CGO), so `go build` produces a static binary that runs on Linux, macOS, and
Windows without any system dependencies. The terminal rendering uses standard ANSI
escape codes via lipgloss/bubbletea.
