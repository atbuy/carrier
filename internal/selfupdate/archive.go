package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// extract returns the contents of the file named name inside a release
// archive: a .tar.gz on Linux and macOS, a .zip on Windows.
func extract(blob []byte, archive, name string) ([]byte, error) {
	if strings.HasSuffix(archive, ".zip") {
		return extractZip(blob, name)
	}
	return extractTarGz(blob, name)
}

func extractTarGz(blob []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, fmt.Errorf("cannot read the downloaded archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read the downloaded archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || path.Base(header.Name) != name {
			continue
		}
		return readBinary(tr, name)
	}
	return nil, fmt.Errorf("the downloaded archive does not contain %s", name)
}

func extractZip(blob []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return nil, fmt.Errorf("cannot read the downloaded archive: %w", err)
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || path.Base(f.Name) != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("cannot read %s from the downloaded archive: %w", name, err)
		}
		defer func() { _ = rc.Close() }()
		return readBinary(rc, name)
	}
	return nil, fmt.Errorf("the downloaded archive does not contain %s", name)
}

// readBinary reads one archive member, refusing an empty or implausibly large
// one rather than staging it over a working binary.
func readBinary(r io.Reader, name string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBinaryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s from the downloaded archive: %w", name, err)
	}
	if len(data) > maxBinaryBytes {
		return nil, fmt.Errorf("%s in the downloaded archive is larger than %d bytes", name, maxBinaryBytes)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s in the downloaded archive is empty", name)
	}
	return data, nil
}
