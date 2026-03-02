# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

<!-- semantic-release manages entries below this line -->

## [0.1.0] - 2026-02-22

### Added
- Initial release of dbx-dash
- Textual TUI dashboard for Databricks workspace monitoring
- Dashboard screen with health cards per workspace
- Jobs screen with sortable, filterable DataTable
- Clusters screen with state/worker/source columns
- SQL Warehouses screen
- Run detail screen with log viewer
- Multi-workspace support via ~/.databrickscfg profiles
- Optional override config at ~/.dbx-dash/config.toml
- SQLite local cache with async repository
- Terminal bell + visual flash alerts on job failures
- Protocol-based API layer for full testability
- `dbx-dash run`, `dbx-dash config list`, `dbx-dash config check` CLI commands

### Dependencies
- Built on [databricks-sdk-go v0.55.0](https://github.com/databricks/databricks-sdk-go)
