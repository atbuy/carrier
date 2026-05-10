# Installation

This page covers supported install paths.

## Install with the script

Linux and macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh
```

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh -s -- --version v0.1.0
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1 | iex
```

Install a specific version on Windows:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1))) -Version v0.1.0"
```

The installer detects your OS and CPU architecture, installs into your user bin directory, and prints `carrier version` when done. Use `--system` on Linux/macOS or `-System` on Windows for a system-wide install.

Release notes include checksum verification details for users who want manual validation.

## Install with Go

If you have Go installed:

```bash
go install github.com/atbuy/carrier/cmd/carrier@latest
```

Ensure Go's bin dir is on `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Verify:

```bash
carrier version
```

## Build from source

```bash
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

```bash
alias c='carrier'
```

Add it to:

- `~/.zshrc` for zsh
- `~/.bashrc` for bash

## Optional desktop notifications

On Ubuntu, `carrier` uses `notify-send` for opt-in notifications.

Check availability:

```bash
carrier doctor
```

Use notifications:

```bash
carrier -n run docker compose build
carrier -N run ls
```
