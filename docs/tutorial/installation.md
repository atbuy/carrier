# Installation

This page covers supported install paths.

## Install with the script

=== "Linux/macOS"

    ```bash title="Install latest"
    curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh
    ```

    ```bash title="Install a specific version"
    curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh -s -- --version v0.1.0
    ```

    ```bash title="Install system-wide"
    curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh -s -- --system
    ```

=== "Windows"

    ```powershell title="Install latest"
    irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1 | iex
    ```

    ```powershell title="Install a specific version"
    powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1))) -Version v0.1.0"
    ```

    ```powershell title="Install system-wide"
    powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1))) -System"
    ```

The installer detects your OS and CPU architecture, installs into your user bin directory, and prints `carrier version` when done. Use `--system` on Linux/macOS or `-System` on Windows for a system-wide install.

!!! tip "Default install locations"

    Linux/macOS installs to `~/.local/bin/carrier` by default. Windows installs to `%LOCALAPPDATA%\carrier\bin\carrier.exe` and adds that directory to the user `PATH`.

Release notes include checksum verification details for users who want manual validation.

## Install with Go

If you have Go installed:

```bash title="Install latest Go module"
go install github.com/atbuy/carrier/cmd/carrier@latest
```

Ensure Go's bin dir is on `PATH`:

```bash title="Add GOPATH/bin to PATH"
export PATH="$PATH:$(go env GOPATH)/bin"
```

Verify:

```bash title="Verify install"
carrier version
carrier doctor
```

## Build from source

```bash title="Build local binary"
git clone https://github.com/atbuy/carrier.git
cd carrier
make build
./bin/carrier version
```

Install from checkout:

```bash
make install
```

## Recommended shell alias

```bash title="Alias"
alias c='carrier'
```

Add it to:

- `~/.zshrc` for zsh
- `~/.bashrc` for bash

## Optional desktop notifications

Notifications are requested per command, not enabled globally. Carrier uses the platform notification tool:

| Platform | Tool                            |
| -------- | ------------------------------- |
| Linux    | `notify-send`                   |
| macOS    | `osascript`                     |
| Windows  | PowerShell notification balloon |

Check availability:

```bash title="Check local setup"
carrier doctor
```

Use notifications:

```bash title="Request notifications"
carrier -n run docker compose build
carrier -N run ls
```

`-n` respects `notify.min_duration`; `-N` bypasses that duration gate.
