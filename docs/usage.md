# Usage

## Commands

### `dbx-dash run`

Launch the dashboard. Reads all workspace profiles from `~/.databrickscfg` by default.

```bash
dbx-dash run [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--workspace <name>` | `-w` | Show only a single workspace |
| `--workspaces <list>` | | Comma-separated list of workspaces to show |
| `--refresh <seconds>` | `-r` | Override the default refresh interval |

**Examples:**

```bash
# All workspaces
dbx-dash run

# Single workspace
dbx-dash run --workspace production

# Multiple workspaces
dbx-dash run --workspaces production,staging

# Custom refresh interval
dbx-dash run --refresh 60
```

### `dbx-dash config list`

Print all workspace profiles found in `~/.databrickscfg`.

```bash
dbx-dash config list
```

### `dbx-dash config check <workspace>`

Test connectivity to a specific workspace. Calls the Databricks API to validate credentials.

```bash
dbx-dash config check production
```

---

## Screens

Switch between screens with the number keys.

### `1` Dashboard

Health summary cards per workspace — running jobs, failed jobs, active clusters.

### `2` Jobs

Sortable table of all jobs. Press `Enter` on a row to open the run detail panel with a scrollable log viewer.

### `3` Clusters

Table of clusters with state, worker counts, node type, Spark version, and source.

### `4` SQL Warehouses

Table of SQL warehouses with state, cluster size, and active sessions.

### `5` Identity

Tree view of groups, users, and service principals.

- Press `Enter` on a group to expand/collapse its members
- Press `Enter` on a user or service principal to open a detail popup showing:
    - Workspace entitlements
    - Roles
    - Group memberships (direct and transitive)
    - Unity Catalog permissions

!!! note
    The Identity screen requires workspace-level access. No account admin is needed.

---

## Keybindings

| Key | Action |
|-----|--------|
| `1` | Dashboard screen |
| `2` | Jobs screen |
| `3` | Clusters screen |
| `4` | SQL Warehouses screen |
| `5` | Identity screen |
| `r` | Refresh current screen |
| `w` | Switch to next workspace |
| `Enter` | Drill into row / expand group |
| `Esc` / `b` | Go back |
| `/` | Search / filter (Identity screen) |
| `q` | Quit |
