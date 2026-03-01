# dbx-dash

Terminal dashboard for monitoring Databricks workspaces, written in Go.

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

- **Multi-workspace** — monitors all profiles from `~/.databrickscfg` simultaneously
- **Live refresh** — auto-polls every 30s (configurable), `r` to force refresh
- **5 screens** — Dashboard, Jobs, Clusters, SQL Warehouses, Identity
- **Drill-down** — select any row to view run detail and log tail
- **Alerts** — terminal bell and visual flash on job failures
- **SQLite cache** — browse recent data even when offline

## Quick start

```bash
# Install
go install github.com/tttao/dbx-dash/cmd/dbx-dash@latest

# Launch (reads all profiles from ~/.databrickscfg)
dbx-dash run
```

See [Installation](install.md) for other install methods and [Usage](usage.md) for all commands.
