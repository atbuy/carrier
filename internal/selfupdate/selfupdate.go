// Package selfupdate downloads a carrier release from GitHub Releases and
// replaces the running binary with it. It is the only part of carrier that
// makes a network request, and only when the user asks for one.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/atbuy/carrier/internal/changelog"
)

const (
	// Repo is the GitHub repository releases are published to.
	Repo = "atbuy/carrier"

	// BaseURLEnv overrides the GitHub base URL. Tests point it at a local
	// server so none of them reach the network.
	BaseURLEnv = "CARRIER_UPDATE_BASE_URL"

	defaultBaseURL = "https://github.com"

	// Archives are a few MB; these caps only stop a runaway download.
	maxArchiveBytes = 200 << 20
	maxBinaryBytes  = 400 << 20
)

// platforms are the GOOS/GOARCH pairs .github/workflows/release.yml builds.
// Checking up front turns an unsupported platform into a clear message
// instead of a 404 for an asset that was never published.
var platforms = map[string]bool{
	"linux/amd64":   true,
	"linux/arm64":   true,
	"darwin/amd64":  true,
	"darwin/arm64":  true,
	"windows/amd64": true,
	"windows/arm64": true,
}

// Updater resolves and installs releases. The zero value is not usable; call
// New.
type Updater struct {
	BaseURL string
	Client  *http.Client
	GOOS    string
	GOARCH  string
}

func New() *Updater {
	base := os.Getenv(BaseURLEnv)
	if base == "" {
		base = defaultBaseURL
	}
	return &Updater{
		BaseURL: strings.TrimSuffix(base, "/"),
		Client:  &http.Client{Timeout: 5 * time.Minute},
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
	}
}

// NormalizeTag turns a version a user typed ("1.3.1") into a release tag
// ("v1.3.1").
func NormalizeTag(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return ""
	}
	return "v" + strings.TrimPrefix(v, "v")
}

// LatestVersion returns the newest release tag, for example "v1.3.1".
//
// It reads the redirect target of the /releases/latest page rather than
// calling api.github.com, whose unauthenticated limit of 60 requests an hour
// per IP fails behind shared addresses and in CI.
func (u *Updater) LatestVersion(ctx context.Context) (string, error) {
	noFollow := *u.Client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	target := fmt.Sprintf("%s/%s/releases/latest", u.BaseURL, Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach %s: %w", u.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("%s did not name a latest release (%s)", target, resp.Status)
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("latest release URL %q: %w", location, err)
	}
	tag := path.Base(parsed.Path)
	if !changelog.IsRelease(tag) {
		return "", fmt.Errorf("latest release URL %q does not end in a version", location)
	}
	return tag, nil
}

// Install downloads the release tag for this platform, verifies it against the
// release's checksums.txt, and replaces the binary at exePath with it. The old
// binary is left in place if anything fails before the final step.
func (u *Updater) Install(ctx context.Context, tag, exePath string) error {
	if !platforms[u.GOOS+"/"+u.GOARCH] {
		return fmt.Errorf("no carrier release is published for %s/%s; build from source instead", u.GOOS, u.GOARCH)
	}
	archive := u.archiveName()

	// Fetch the checksum first: a tag that does not exist fails here, before
	// spending a download on it.
	want, err := u.checksum(ctx, tag, archive)
	if err != nil {
		return err
	}
	blob, err := u.fetch(ctx, u.assetURL(tag, archive))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(blob)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s (expected %s, got %s); nothing was installed", archive, want, got)
	}
	binary, err := extract(blob, archive, u.binaryName())
	if err != nil {
		return err
	}

	mode := os.FileMode(0o755)
	if info, err := os.Stat(exePath); err == nil {
		mode = info.Mode().Perm()
	}
	staged, err := stage(filepath.Dir(exePath), binary, mode)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(staged) }()
	return replace(staged, exePath)
}

// stage writes the new binary next to the one it will replace, so the final
// rename stays within one filesystem and is atomic.
func stage(dir string, binary []byte, mode os.FileMode) (string, error) {
	f, err := os.CreateTemp(dir, ".carrier-update-*")
	if err != nil {
		return "", fmt.Errorf("cannot write to %s: %w", dir, err)
	}
	name := f.Name()
	write := func() error {
		if _, err := f.Write(binary); err != nil {
			return err
		}
		if err := f.Chmod(mode); err != nil {
			return err
		}
		return f.Close()
	}
	if err := write(); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("cannot write the new binary to %s: %w", dir, err)
	}
	return name, nil
}

// checksum returns the recorded sha256 of archive in the release's
// checksums.txt, which is written by sha256sum as "<hash>  <file>".
func (u *Updater) checksum(ctx context.Context, tag, archive string) (string, error) {
	blob, err := u.fetch(ctx, u.assetURL(tag, "checksums.txt"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(blob), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == archive {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("release %s publishes no checksum for %s", tag, archive)
}

func (u *Updater) assetURL(tag, asset string) string {
	return fmt.Sprintf("%s/%s/releases/download/%s/%s", u.BaseURL, Repo, tag, asset)
}

// archiveName is the release asset for this platform, as packaged by
// .github/workflows/release.yml.
func (u *Updater) archiveName() string {
	if u.GOOS == "windows" {
		return fmt.Sprintf("carrier-windows-%s.zip", u.GOARCH)
	}
	return fmt.Sprintf("carrier-%s-%s.tar.gz", u.GOOS, u.GOARCH)
}

// binaryName is the file inside the archive, which carries the platform in
// its name rather than being called "carrier".
func (u *Updater) binaryName() string {
	name := fmt.Sprintf("carrier-%s-%s", u.GOOS, u.GOARCH)
	if u.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func (u *Updater) fetch(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot download %s: %w", path.Base(target), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s was not found; check the release exists at %s/%s/releases", path.Base(target), u.BaseURL, Repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cannot download %s: %s", path.Base(target), resp.Status)
	}
	blob, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot download %s: %w", path.Base(target), err)
	}
	if len(blob) > maxArchiveBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", path.Base(target), maxArchiveBytes)
	}
	return blob, nil
}

// Target is the binary a self update would replace.
type Target struct {
	// Path is the resolved path of the running binary.
	Path string
	// GoBin reports that it sits where "go install" puts binaries, so it
	// should be updated with go install rather than from a release archive.
	GoBin bool
}

// Dir is the directory holding the binary.
func (t Target) Dir() string { return filepath.Dir(t.Path) }

// Locate finds the running binary.
func Locate() (Target, error) {
	exe, err := os.Executable()
	if err != nil {
		return Target{}, fmt.Errorf("cannot find the running carrier binary: %w", err)
	}
	// Replace the real file, not a symlink pointing at it.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	if _, err := os.Stat(exe); err != nil {
		return Target{}, fmt.Errorf("cannot find the running carrier binary at %s: %w", exe, err)
	}
	return Target{Path: exe, GoBin: inGoBin(exe)}, nil
}

// inGoBin reports whether path sits in the directory "go install" writes to.
// GOBIN, GOPATH and the default ~/go are read from the environment, so this
// works without "go" on PATH.
func inGoBin(target string) bool {
	var dirs []string
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		dirs = append(dirs, gobin)
	}
	for _, gopath := range filepath.SplitList(os.Getenv("GOPATH")) {
		if gopath != "" {
			dirs = append(dirs, filepath.Join(gopath, "bin"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "go", "bin"))
	}
	// Compare resolved paths: a temp or home directory can be reached through
	// a symlink (macOS /var is /private/var) or a short 8.3 name on Windows.
	resolve := func(path string) string {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return resolved
		}
		return filepath.Clean(path)
	}
	dir := resolve(filepath.Dir(target))
	for _, candidate := range dirs {
		if samePath(dir, resolve(candidate)) {
			return true
		}
	}
	return false
}

// samePath compares two cleaned paths, ignoring case on Windows, whose file
// systems do.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
