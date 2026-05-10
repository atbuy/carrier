# Release install reference

Release artifacts are published on GitHub:

```text
https://github.com/atbuy/carrier/releases
```

## Artifact names

Linux:

```text
carrier-linux-amd64.tar.gz
carrier-linux-arm64.tar.gz
```

macOS:

```text
carrier-darwin-amd64.tar.gz
carrier-darwin-arm64.tar.gz
```

Windows:

```text
carrier-windows-amd64.zip
carrier-windows-arm64.zip
```

Checksums:

```text
checksums.txt
```

## Verify with checksums.txt

```bash
sha256sum --check --ignore-missing checksums.txt
```

## Verify with GitHub asset digest

GitHub shows a digest beside release assets:

```text
sha256:<hash>
```

Compare that value to your locally computed hash:

```bash
sha256sum carrier-linux-amd64.tar.gz
```

## Install Linux archive

```bash
version=v0.1.0
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/carrier-linux-amd64.tar.gz"
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar -xzf carrier-linux-amd64.tar.gz
install -Dm755 carrier-linux-amd64 ~/.local/bin/carrier
```

## Install macOS archive

```bash
version=v0.1.0
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/carrier-darwin-arm64.tar.gz"
tar -xzf carrier-darwin-arm64.tar.gz
install -m 755 carrier-darwin-arm64 /usr/local/bin/carrier
```

## Install Windows archive

Download the `.zip` file, extract `carrier.exe`, and place it in a directory on `PATH`.

PowerShell hash check:

```powershell
Get-FileHash .\carrier-windows-amd64.zip -Algorithm SHA256
```
