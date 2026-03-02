# dbx-dash

Terminal dashboard for monitoring Databricks workspaces, written in Go.

![dbx-dash screenshot](docs/Screenshot%202026-03-02%20at%2012.16.07.png)

```
┌─ dbx-dash v0.2.0 ──────────────────────────────────────── 14:23:01 ─┐
│ Workspace: ◀  production  ▶                          ⟳ refreshing…  │
├───────────────────────────────────────────────────────────────────────┤
│  production                                                           │
│  ┌─Running Jobs──┐ ┌─Failed Jobs───┐ ┌─Active Clusters─┐            │
│  │      3        │ │      1        │ │       4         │            │
│  └───────────────┘ └───────────────┘ └─────────────────┘            │
│                                                                       │
│  staging                                                              │
│  ┌─Running Jobs──┐ ┌─Failed Jobs───┐ ┌─Active Clusters─┐            │
│  │      0        │ │      0        │ │       1         │            │
│  └───────────────┘ └───────────────┘ └─────────────────┘            │
├───────────────────────────────────────────────────────────────────────┤
│ 1 Dashboard  2 Jobs  3 Clusters  4 Warehouses  r Refresh  q Quit    │
└───────────────────────────────────────────────────────────────────────┘
```

## Features

- **Multi-workspace**: monitors all profiles from `~/.databrickscfg` simultaneously
- **Live refresh**: auto-polls every 30s (configurable), `r` to force refresh
- **4 screens**: Dashboard, Jobs, Clusters, SQL Warehouses
- **Drill-down**: select any job row to view run detail and log tail
- **Alerts**: terminal bell and visual flash on job failures
- **SQLite cache**: browse recent data even when offline
- **Interface-driven**: data layer separated behind Go interfaces (swappable SDK, REST, or mock)

## Install

```bash
go install github.com/tttao/dbx-dash/cmd/dbx-dash@latest
```

Or build from source:

```bash
git clone https://github.com/tttao/dbx-dash
cd dbx-dash
go build -o dbx-dash ./cmd/dbx-dash
```

## Usage

```bash
# Launch dashboard (reads all profiles from ~/.databrickscfg)
dbx-dash run

# Start on a specific workspace
dbx-dash run --workspace production

# Custom refresh interval (seconds)
dbx-dash run --refresh 60

# Filter to specific workspaces
dbx-dash run --workspaces production,staging

# List configured workspaces
dbx-dash config list

# Test connectivity to a workspace
dbx-dash config check production
```

## Configuration

dbx-dash reads `~/.databrickscfg` automatically (same format as Databricks CLI).

For display name overrides and per-workspace refresh intervals, create `~/.dbx-dash/config.toml`:

```toml
[[workspace]]
name = "production"
display_name = "Prod (EU-West)"
refresh_interval = 60

[[workspace]]
name = "staging"
display_name = "Staging"
refresh_interval = 30

[refresh]
default_interval = 30
alert_on_failure = true
```

## Keybindings

| Key | Action |
|-----|--------|
| `1` | Dashboard |
| `2` | Jobs |
| `3` | Clusters |
| `4` | SQL Warehouses |
| `r` | Refresh |
| `w` | Next workspace |
| `enter` | Drill into row |
| `esc` / `b` | Back |
| `q` | Quit |

---

## Architecture

### Project layout

```
dbx-dash/
├── cmd/
│   └── dbx-dash/
│       └── main.go               # entry point: cobra CLI → tui.Run()
├── internal/
│   ├── config/
│   │   ├── model.go              # WorkspaceProfile, AppConfig, RefreshConfig
│   │   └── loader.go             # parse ~/.databrickscfg (INI) + ~/.dbx-dash/config.toml
│   ├── databricks/
│   │   ├── interfaces.go         # JobsProvider, ClustersProvider, WarehousesProvider, PipelinesProvider
│   │   ├── models.go             # Job, JobRun, RunOutput, Cluster, SqlWarehouse, Pipeline, PipelineUpdate
│   │   ├── sdk/
│   │   │   ├── factory.go        # WorkspaceClientFactory: lazy init, sync.Map cache
│   │   │   ├── jobs.go           # SDKJobsProvider (databricks-sdk-go)
│   │   │   ├── clusters.go       # SDKClustersProvider
│   │   │   ├── warehouses.go     # SDKWarehousesProvider
│   │   │   └── pipelines.go      # SDKPipelinesProvider
│   │   └── mock/
│   │       └── providers.go      # MockJobsProvider, etc. (used in tests)
│   ├── cache/
│   │   ├── db.go                 # SQLite init, schema (modernc.org/sqlite, no CGO)
│   │   └── repository.go         # UpsertJob, GetJobs, UpsertCluster, GetClusters
│   ├── tui/
│   │   ├── app.go                # root bubbletea Model, Update, View
│   │   ├── keys.go               # keyMap definitions
│   │   ├── styles.go             # lipgloss style constants
│   │   └── screens/
│   │       ├── dashboard.go      # health-card summaries per workspace
│   │       ├── jobs.go           # sortable jobs table
│   │       ├── clusters.go       # clusters table
│   │       ├── warehouses.go     # SQL warehouses table
│   │       └── run_detail.go     # job run detail + scrollable log viewer
│   └── alerts/
│       └── terminal.go           # RingBell(), FlashAlert()
├── go.mod
└── README.md
```

### Interface layer

All Databricks data access is behind Go interfaces defined in `internal/databricks/interfaces.go`:

```go
type JobsProvider interface {
    ListJobs(ctx context.Context) ([]Job, error)
    ListRuns(ctx context.Context, jobID int64, limit int) ([]JobRun, error)
    GetRunOutput(ctx context.Context, runID int64) (*RunOutput, error)
}

type ClustersProvider interface {
    ListClusters(ctx context.Context) ([]Cluster, error)
}

type WarehousesProvider interface {
    ListWarehouses(ctx context.Context) ([]SqlWarehouse, error)
}

type PipelinesProvider interface {
    ListPipelines(ctx context.Context) ([]Pipeline, error)
    ListUpdates(ctx context.Context, pipelineID string, limit int) ([]PipelineUpdate, error)
}

// WorkspaceProviders bundles all four providers for one workspace.
type WorkspaceProviders struct {
    Jobs       JobsProvider
    Clusters   ClustersProvider
    Warehouses WarehousesProvider
    Pipelines  PipelinesProvider
}
```

The default production implementation lives in `internal/databricks/sdk/` and wraps
`github.com/databricks/databricks-sdk-go`. Tests inject `mock.MockJobsProvider` etc.
A future REST implementation can be dropped in without touching TUI or cache code.

### Domain models

```go
// internal/databricks/models.go

type Job struct {
    JobID       int64
    Name        string
    Creator     string
    CreatedTime time.Time
}

type JobRun struct {
    RunID       int64
    JobID       int64
    State       string // PENDING, RUNNING, TERMINATING, TERMINATED, SKIPPED, INTERNAL_ERROR
    ResultState string // SUCCESS, FAILED, TIMEDOUT, CANCELED
    StartTime   time.Time
    EndTime     time.Time
    RunPageURL  string
}

type RunOutput struct {
    RunID      int64
    Metadata   JobRun
    Error      string
    ErrorTrace string
    Logs       string
}

type Cluster struct {
    ClusterID    string
    Name         string
    State        string
    NumWorkers   int
    MinWorkers   int
    MaxWorkers   int
    NodeTypeID   string
    DriverTypeID string
    SparkVersion string
    Source       string
}

type SqlWarehouse struct {
    WarehouseID    string
    Name           string
    State          string
    ClusterSize    string
    NumClusters    int
    ActiveSessions int
}

type Pipeline struct {
    PipelineID   string
    Name         string
    State        string
    ClusterID    string
    Creator      string
    LastModified time.Time
}

type PipelineUpdate struct {
    UpdateID    string
    PipelineID  string
    State       string
    FullRefresh bool
    Cause       string
}
```

### SDK factory (lazy init)

```go
// internal/databricks/sdk/factory.go

type WorkspaceClientFactory struct {
    clients sync.Map // key: profile name, value: *databricks.WorkspaceClient
}

func (f *WorkspaceClientFactory) GetProviders(p config.WorkspaceProfile) (*databricks.WorkspaceProviders, error) {
    client, err := f.getOrCreate(p)
    if err != nil {
        return nil, err
    }
    return &databricks.WorkspaceProviders{
        Jobs:       &SDKJobsProvider{client: client},
        Clusters:   &SDKClustersProvider{client: client},
        Warehouses: &SDKWarehousesProvider{client: client},
        Pipelines:  &SDKPipelinesProvider{client: client},
    }, nil
}
```

### TUI (Bubble Tea)

The root `Model` in `internal/tui/app.go` manages screen state and delegates `Update`/`View`
to the active screen model. Async data loading uses `tea.Cmd`:

```go
func loadJobsCmd(ctx context.Context, ws string, p databricks.JobsProvider) tea.Cmd {
    return func() tea.Msg {
        jobs, err := p.ListJobs(ctx)
        return jobsLoadedMsg{workspace: ws, jobs: jobs, err: err}
    }
}
```

Multi-workspace parallel fetches use `tea.Batch(cmd1, cmd2, ...)`.

### Cache (SQLite, no CGO)

`modernc.org/sqlite` (pure Go) stores `job_snapshots` and `cluster_snapshots`. Schema is
versioned via `PRAGMA user_version`. The cache is read when a workspace is unreachable.

---

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/databricks/databricks-sdk-go` | Databricks API client |
| `github.com/charmbracelet/bubbletea` | TUI event loop |
| `github.com/charmbracelet/lipgloss` | Terminal styling |
| `github.com/charmbracelet/bubbles` | Table, spinner, viewport widgets |
| `github.com/BurntSushi/toml` | `~/.dbx-dash/config.toml` parsing |
| `gopkg.in/ini.v1` | `~/.databrickscfg` (INI) parsing |
| `modernc.org/sqlite` | SQLite cache (pure Go, no CGO) |
| `github.com/spf13/cobra` | CLI commands |

---

## Development

```bash
# Build
go build ./...

# Run tests
go test ./...

# Vet + lint
go vet ./...

# Run dashboard locally
go run ./cmd/dbx-dash run
```
