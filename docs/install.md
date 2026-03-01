# Installation

## Requirements

- Go 1.22+ (for `go install`)
- A `~/.databrickscfg` with at least one workspace profile

## go install (recommended)

```bash
go install github.com/tttao/dbx-dash/cmd/dbx-dash@latest
```

The binary is placed in `$GOPATH/bin` (usually `~/go/bin`). Make sure it's on your `$PATH`.

## Pre-built binaries

Download the latest release for your platform from the [Releases page](https://github.com/tttao/dbx-dash/releases).

| Platform | File |
|----------|------|
| macOS (Apple Silicon) | `dbx-dash_VERSION_darwin_arm64.tar.gz` |
| macOS (Intel) | `dbx-dash_VERSION_darwin_amd64.tar.gz` |
| Linux (x86-64) | `dbx-dash_VERSION_linux_amd64.tar.gz` |
| Linux (ARM64) | `dbx-dash_VERSION_linux_arm64.tar.gz` |
| Windows (x86-64) | `dbx-dash_VERSION_windows_amd64.zip` |

Extract and place the `dbx-dash` binary somewhere on your `$PATH`.

## Build from source

```bash
git clone https://github.com/tttao/dbx-dash
cd dbx-dash
make install
```

This builds with the current git tag as the version string and installs to `$GOPATH/bin`.

## Verify

```bash
dbx-dash --version
```
