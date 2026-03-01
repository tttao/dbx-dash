# Configuration

## Databricks credentials — `~/.databrickscfg`

dbx-dash reads workspace profiles from the standard Databricks CLI config file. No extra setup is needed if you already use the Databricks CLI.

```ini
[DEFAULT]
host = https://adb-xxxx.azuredatabricks.net
token = dapi...

[production]
host  = https://adb-1234.azuredatabricks.net
token = dapi...

[staging]
host  = https://adb-5678.azuredatabricks.net
client_id     = <service-principal-client-id>
client_secret = <service-principal-client-secret>
```

Supported auth types:

| Type | Fields |
|------|--------|
| Personal Access Token (PAT) | `host` + `token` |
| OAuth M2M | `host` + `client_id` + `client_secret` |

At startup dbx-dash validates each visible workspace. If a profile fails auth, you are prompted to re-authenticate via `databricks auth login`.

---

## App config — `~/.dbx-dash/config.toml`

Optional. Override display names, set per-workspace refresh intervals, and tune alert behaviour.

```toml
[[workspace]]
name            = "production"
display_name    = "Prod (EU-West)"
refresh_interval = 60

[[workspace]]
name            = "staging"
display_name    = "Staging"
refresh_interval = 30

[refresh]
default_interval = 30
alert_on_failure = true

db_path = "~/.dbx-dash/cache.db"
```

### `[[workspace]]`

One block per workspace you want to customise. The `name` field must match a profile name in `~/.databrickscfg`.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Profile name from `~/.databrickscfg` |
| `display_name` | string | Label shown in the dashboard header |
| `refresh_interval` | int | Poll interval in seconds for this workspace |

### `[refresh]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_interval` | int | `30` | Global refresh interval (seconds) |
| `alert_on_failure` | bool | `false` | Ring terminal bell and flash screen on job failures |

### Top-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `db_path` | string | `~/.dbx-dash/cache.db` | Path to the SQLite cache file |

---

## Environment variables

The Databricks SDK also reads standard environment variables, which take precedence over `~/.databrickscfg`:

| Variable | Description |
|----------|-------------|
| `DATABRICKS_HOST` | Workspace URL |
| `DATABRICKS_TOKEN` | PAT |
| `DATABRICKS_CLIENT_ID` | OAuth M2M client ID |
| `DATABRICKS_CLIENT_SECRET` | OAuth M2M client secret |
