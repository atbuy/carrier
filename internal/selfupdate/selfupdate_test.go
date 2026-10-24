package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	testTag    = "v9.9.9"
	newBinary  = "#!/bin/sh\necho carrier 9.9.9\n"
	oldBinary  = "#!/bin/sh\necho carrier 1.0.0\n"
	binaryMode = os.FileMode(0o755)
)

// fakeRelease serves the parts of GitHub Releases that selfupdate uses: the
// /releases/latest redirect, the platform archive, and checksums.txt.
type fakeRelease struct {
	t *testing.T

	tag      string
	archives map[string][]byte // asset name -> archive bytes
	// checksums overrides the recorded hash of an asset, to test a mismatch.
	checksums map[string]string
	// omitChecksum leaves an asset out of checksums.txt entirely.
	omitChecksum bool
	// noLatest serves /releases/latest without a Location header.
	noLatest bool
}

func newFakeRelease(t *testing.T) *fakeRelease {
	t.Helper()
	return &fakeRelease{
		t:         t,
		tag:       testTag,
		archives:  map[string][]byte{},
		checksums: map[string]string{},
	}
}

// addPlatform publishes an archive carrying body as the carrier binary for
// goos/goarch.
func (f *fakeRelease) addPlatform(goos, goarch, body string) {
	f.t.Helper()
	u := &Updater{GOOS: goos, GOARCH: goarch}
	name := u.archiveName()
	if goos == "windows" {
		f.archives[name] = zipArchive(f.t, u.binaryName(), body)
		return
	}
	f.archives[name] = tarGzArchive(f.t, u.binaryName(), body)
}

func (f *fakeRelease) start() *Updater {
	f.t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.handle))
	f.t.Cleanup(srv.Close)

	u := New()
	u.BaseURL = srv.URL
	return u
}

func (f *fakeRelease) handle(w http.ResponseWriter, r *http.Request) {
	prefix := "/" + Repo + "/releases/"
	path := strings.TrimPrefix(r.URL.Path, prefix)
	switch {
	case path == "latest":
		if f.noLatest {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Location", prefix+"tag/"+f.tag)
		w.WriteHeader(http.StatusFound)
	case path == "download/"+f.tag+"/checksums.txt":
		var b strings.Builder
		for name, blob := range f.archives {
			if f.omitChecksum {
				continue
			}
			sum, ok := f.checksums[name]
			if !ok {
				digest := sha256.Sum256(blob)
				sum = hex.EncodeToString(digest[:])
			}
			fmt.Fprintf(&b, "%s  %s\n", sum, name)
		}
		_, _ = w.Write([]byte(b.String()))
	case strings.HasPrefix(path, "download/"+f.tag+"/"):
		name := strings.TrimPrefix(path, "download/"+f.tag+"/")
		blob, ok := f.archives[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(blob)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func tarGzArchive(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func zipArchive(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := f.Write([]byte(body)); err != nil {
		t.Fatalf("write zip body: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// installedBinary writes a stand-in for an installed carrier binary and
// returns its path.
func installedBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "carrier")
	if err := os.WriteFile(path, []byte(oldBinary), binaryMode); err != nil {
		t.Fatalf("write installed binary: %v", err)
	}
	return path
}

func requireContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}

func TestLatestVersion(t *testing.T) {
	f := newFakeRelease(t)
	u := f.start()

	got, err := u.LatestVersion(context.Background())
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	if got != testTag {
		t.Fatalf("LatestVersion = %q, want %q", got, testTag)
	}
}

func TestLatestVersionWithoutRedirect(t *testing.T) {
	f := newFakeRelease(t)
	f.noLatest = true
	u := f.start()

	if _, err := u.LatestVersion(context.Background()); err == nil {
		t.Fatal("LatestVersion without a redirect should fail")
	}
}

func TestLatestVersionRejectsNonVersionTag(t *testing.T) {
	f := newFakeRelease(t)
	f.tag = "nightly"
	u := f.start()

	_, err := u.LatestVersion(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not end in a version") {
		t.Fatalf("LatestVersion error = %v, want a version complaint", err)
	}
}

func TestInstallReplacesBinary(t *testing.T) {
	f := newFakeRelease(t)
	f.addPlatform(runtime.GOOS, runtime.GOARCH, newBinary)
	u := f.start()

	exe := installedBinary(t)
	if err := u.Install(context.Background(), testTag, exe); err != nil {
		t.Fatalf("Install: %v", err)
	}
	requireContent(t, exe, newBinary)

	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("stat %s: %v", exe, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != binaryMode {
		t.Fatalf("mode after install = %v, want %v", info.Mode().Perm(), binaryMode)
	}
	// Nothing is left behind next to the binary.
	entries, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("directory after install holds %d entries, want only the binary", len(entries))
	}
}

func TestInstallRefusesChecksumMismatch(t *testing.T) {
	f := newFakeRelease(t)
	f.addPlatform(runtime.GOOS, runtime.GOARCH, newBinary)
	u := New()
	f.checksums[u.archiveName()] = strings.Repeat("0", 64)
	u = f.start()

	exe := installedBinary(t)
	err := u.Install(context.Background(), testTag, exe)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Install error = %v, want a checksum mismatch", err)
	}
	// The working binary survives a bad download.
	requireContent(t, exe, oldBinary)
}

func TestInstallWithoutChecksumEntry(t *testing.T) {
	f := newFakeRelease(t)
	f.addPlatform(runtime.GOOS, runtime.GOARCH, newBinary)
	f.omitChecksum = true
	u := f.start()

	exe := installedBinary(t)
	err := u.Install(context.Background(), testTag, exe)
	if err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Fatalf("Install error = %v, want a missing checksum", err)
	}
	requireContent(t, exe, oldBinary)
}

func TestInstallMissingAsset(t *testing.T) {
	f := newFakeRelease(t)
	// Publish a different platform, so checksums.txt exists but ours does not.
	other := "amd64"
	if runtime.GOARCH == "amd64" {
		other = "arm64"
	}
	f.addPlatform(runtime.GOOS, other, newBinary)
	u := f.start()

	exe := installedBinary(t)
	if err := u.Install(context.Background(), testTag, exe); err == nil {
		t.Fatal("Install with no asset for this platform should fail")
	}
	requireContent(t, exe, oldBinary)
}

func TestInstallUnsupportedPlatform(t *testing.T) {
	f := newFakeRelease(t)
	u := f.start()
	u.GOOS, u.GOARCH = "plan9", "mips"

	err := u.Install(context.Background(), testTag, installedBinary(t))
	if err == nil || !strings.Contains(err.Error(), "plan9/mips") {
		t.Fatalf("Install error = %v, want an unsupported platform", err)
	}
}

func TestInstallUnknownTag(t *testing.T) {
	f := newFakeRelease(t)
	f.addPlatform(runtime.GOOS, runtime.GOARCH, newBinary)
	u := f.start()

	exe := installedBinary(t)
	err := u.Install(context.Background(), "v0.0.1", exe)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Install error = %v, want a not-found release", err)
	}
	requireContent(t, exe, oldBinary)
}

func TestInstallIntoReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix directory permissions")
	}
	f := newFakeRelease(t)
	f.addPlatform(runtime.GOOS, runtime.GOARCH, newBinary)
	u := f.start()

	exe := installedBinary(t)
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := u.Install(context.Background(), testTag, exe)
	if err == nil || !strings.Contains(err.Error(), "cannot write to") {
		t.Fatalf("Install error = %v, want a write failure", err)
	}
	requireContent(t, exe, oldBinary)
}

func TestExtractArchives(t *testing.T) {
	t.Run("tar.gz", func(t *testing.T) {
		blob := tarGzArchive(t, "carrier-linux-amd64", newBinary)
		got, err := extract(blob, "carrier-linux-amd64.tar.gz", "carrier-linux-amd64")
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if string(got) != newBinary {
			t.Fatalf("extract = %q, want %q", got, newBinary)
		}
	})
	t.Run("zip", func(t *testing.T) {
		blob := zipArchive(t, "carrier-windows-amd64.exe", newBinary)
		got, err := extract(blob, "carrier-windows-amd64.zip", "carrier-windows-amd64.exe")
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if string(got) != newBinary {
			t.Fatalf("extract = %q, want %q", got, newBinary)
		}
	})
	t.Run("missing member", func(t *testing.T) {
		blob := tarGzArchive(t, "something-else", newBinary)
		_, err := extract(blob, "carrier-linux-amd64.tar.gz", "carrier-linux-amd64")
		if err == nil || !strings.Contains(err.Error(), "does not contain") {
			t.Fatalf("extract error = %v, want a missing member", err)
		}
	})
	t.Run("empty member", func(t *testing.T) {
		blob := tarGzArchive(t, "carrier-linux-amd64", "")
		_, err := extract(blob, "carrier-linux-amd64.tar.gz", "carrier-linux-amd64")
		if err == nil || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("extract error = %v, want an empty member", err)
		}
	})
	t.Run("not an archive", func(t *testing.T) {
		_, err := extract([]byte("404: not found"), "carrier-linux-amd64.tar.gz", "carrier-linux-amd64")
		if err == nil {
			t.Fatal("extract of a non-archive should fail")
		}
	})
}

func TestAssetNames(t *testing.T) {
	cases := []struct {
		goos, goarch    string
		archive, binary string
	}{
		{"linux", "amd64", "carrier-linux-amd64.tar.gz", "carrier-linux-amd64"},
		{"darwin", "arm64", "carrier-darwin-arm64.tar.gz", "carrier-darwin-arm64"},
		{"windows", "amd64", "carrier-windows-amd64.zip", "carrier-windows-amd64.exe"},
	}
	for _, tc := range cases {
		u := &Updater{GOOS: tc.goos, GOARCH: tc.goarch}
		if got := u.archiveName(); got != tc.archive {
			t.Errorf("archiveName(%s/%s) = %q, want %q", tc.goos, tc.goarch, got, tc.archive)
		}
		if got := u.binaryName(); got != tc.binary {
			t.Errorf("binaryName(%s/%s) = %q, want %q", tc.goos, tc.goarch, got, tc.binary)
		}
	}
}

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"1.3.1":  "v1.3.1",
		"v1.3.1": "v1.3.1",
		" 1.3.1": "v1.3.1",
		"":       "",
	}
	for in, want := range cases {
		if got := NormalizeTag(in); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocate(t *testing.T) {
	target, err := Locate()
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if !filepath.IsAbs(target.Path) {
		t.Fatalf("Locate path = %q, want an absolute path", target.Path)
	}
	if target.Dir() != filepath.Dir(target.Path) {
		t.Fatalf("Dir() = %q, want %q", target.Dir(), filepath.Dir(target.Path))
	}
}

func TestInGoBin(t *testing.T) {
	gobin := t.TempDir()
	gopath := t.TempDir()
	t.Setenv("GOBIN", gobin)
	t.Setenv("GOPATH", gopath)

	cases := map[string]bool{
		filepath.Join(gobin, "carrier"):                  true,
		filepath.Join(gopath, "bin", "carrier"):          true,
		filepath.Join(gopath, "carrier"):                 false,
		filepath.Join(t.TempDir(), "carrier"):            false,
		filepath.Join("/usr", "local", "bin", "carrier"): false,
	}
	for path, want := range cases {
		if got := inGoBin(path); got != want {
			t.Errorf("inGoBin(%q) = %v, want %v", path, got, want)
		}
	}

	// The binary reached through a symlink to GOBIN is still in GOBIN.
	link := filepath.Join(t.TempDir(), "gobin-link")
	if err := os.Symlink(gobin, link); err != nil {
		t.Skipf("cannot create a symlink: %v", err)
	}
	if !inGoBin(filepath.Join(link, "carrier")) {
		t.Errorf("inGoBin through a symlink to GOBIN = false, want true")
	}
}

func TestBaseURLFromEnv(t *testing.T) {
	t.Setenv(BaseURLEnv, "http://127.0.0.1:1/")
	if got := New().BaseURL; got != "http://127.0.0.1:1" {
		t.Fatalf("BaseURL = %q, want the environment value without a trailing slash", got)
	}
}
