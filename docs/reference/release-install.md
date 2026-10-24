# Release install reference

Release artifacts are published on [GitHub Releases](https://github.com/atbuy/carrier/releases).

## Install scripts

Latest Linux/macOS release:

```bash
curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh
```

Specific Linux/macOS release:

```bash
curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh -s -- --version v1.0.0
```

Latest Windows release:

```powershell
irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1 | iex
```

Specific Windows release:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1))) -Version v1.0.0"
```

## Updating

A carrier installed from a release archive updates itself:

```bash title="Update to the latest release"
carrier self update
```

```bash title="See what is available first"
carrier self update --check
```

It resolves the latest tag, downloads the archive for this OS and architecture, checks it against the release's `checksums.txt`, and replaces the running binary. The old binary stays in place if any step fails, and shells that are already recording keep working.

Installs that came from somewhere else are left alone, and the command says what to run instead: `go install github.com/atbuy/carrier/cmd/carrier@latest` for a Go install, `make install` for a build from source, and the `--system` installer line for a directory this user cannot write. See [`carrier self update`](../guide/commands.md#self-update).

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

## Manual checksum validation

Release notes include the short checksum workflow for each release. Manual validation uses `checksums.txt`:

```bash
curl -LO "https://github.com/atbuy/carrier/releases/download/v1.0.0/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
```

GitHub also shows a `sha256:<hash>` digest beside each release asset. Compare it with:

```bash
sha256sum carrier-linux-amd64.tar.gz
```

## Manual archive install

Download the archive for your OS and architecture from GitHub Releases, extract it, and place the binary on `PATH`.

```bash
version=v1.0.0
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/carrier-linux-amd64.tar.gz"
tar -xzf carrier-linux-amd64.tar.gz
install -Dm755 carrier-linux-amd64 ~/.local/bin/carrier
```

Windows archives contain `carrier.exe`; copy it to a directory on `PATH`.

```powershell
Expand-Archive .\carrier-windows-amd64.zip
```

## Maintainer release workflow

Publishing a `v*` tag builds Linux, macOS, and Windows assets, then publishes a GitHub release with generated release notes and `checksums.txt`.

Every release needs an entry in `CHANGELOG.md`:

```markdown
## [0.30.0] - 2026-11-02

### Added

- What users can do now that they could not before.

### Fixed

- What no longer goes wrong.
```

Add the entry in the change that will ship in that version. Use the sections `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, and `Security`, newest version first. The release workflow fails before building when the tag has no entry, and the entry becomes the top of the GitHub release notes. `go test .` checks that the file is well formed.

```bash
git tag v1.0.0
git push origin v1.0.0
```

The release workflow also supports manual dispatch for rebuilding assets from an existing tag. Set the `tag` input to the release tag, leave `ref` empty to build that tag, and use the draft or prerelease inputs when needed.
