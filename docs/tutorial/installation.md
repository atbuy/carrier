# Installation

This page covers supported install paths.

## Install from GitHub Releases

Download prebuilt binaries from:

```text
https://github.com/atbuy/carrier/releases
```

Linux x86_64 example:

```bash
version=v0.1.0
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/carrier-linux-amd64.tar.gz"
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar -xzf carrier-linux-amd64.tar.gz
install -Dm755 carrier-linux-amd64 ~/.local/bin/carrier
carrier version
```

macOS arm64 example:

```bash
version=v0.1.0
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/carrier-darwin-arm64.tar.gz"
tar -xzf carrier-darwin-arm64.tar.gz
install -m 755 carrier-darwin-arm64 /usr/local/bin/carrier
carrier version
```

Windows users can download the `.zip` archive for their architecture and place `carrier.exe` on `PATH`.

## Verify checksums

Release uploads include `checksums.txt`.

```bash
sha256sum --check --ignore-missing checksums.txt
```

GitHub also shows a `sha256:<hash>` digest beside each uploaded release asset.

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
