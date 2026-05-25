// Package cache is a tiny atomic-write helper for the local cache files
// clarity's adapters keep under .git/clarity/. It deliberately doesn't
// know what's in the bytes — callers JSON-marshal whatever they want and
// hand it over. Writes go through a temp-file + rename so a crash mid-
// write can't leave a torn payload on disk.
package cache

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// File is the cache helper bound to a single on-disk path. Constructed
// once per cache slot (e.g. snapshot-cache.json.gz) and reused for every
// Read/Write/Invalidate against that path.
type File struct {
	path string
}

// New returns a File pointing at path. The file does not have to exist
// yet — Read on a missing path returns (nil, false, nil), and Write
// creates parent directories as needed.
func New(path string) *File { return &File{path: path} }

// Read returns (decompressedBytes, true, nil) when the cache exists and
// is valid, (nil, false, nil) when the file is absent, and (nil, false,
// err) for any other failure (corruption, gzip error, permission denied).
// Callers fall back to a cold-start path when exists is false.
func (f *File) Read() ([]byte, bool, error) {
	raw, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, false, err
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// Write gzip-compresses data and writes it to the configured path,
// creating parent directories as needed. The on-disk file is replaced
// atomically: bytes go to a sibling temp file first, then a rename swaps
// it into place — so a crash mid-write can't leave a half-written cache
// that a concurrent Read would observe.
func (f *File) Write(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), filepath.Base(f.path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if any step below fails before the rename.
	defer func() { _ = os.Remove(tmpPath) }()

	zw := gzip.NewWriter(tmp)
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, f.path)
}

// Invalidate removes the cache file. Safe to call when the file doesn't
// exist — callers that "clear cache on startup if config changed"
// shouldn't have to stat first.
func (f *File) Invalidate() error {
	err := os.Remove(f.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
