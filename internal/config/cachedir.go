package config

import "path/filepath"

// CacheDirSources are the three places a cache-dir override might come
// from, in precedence order: flag wins over env wins over the default
// <repoRoot>/.git/clarity path. Grouped into one struct so the caller
// can resolve them in one call instead of nesting if-elses at every
// call site.
type CacheDirSources struct {
	// Flag is the value passed to --cache-dir, or "" when the flag
	// wasn't given.
	Flag string
	// Env is the value of $CLARITY_CACHE_DIR, or "" when unset.
	Env string
	// RepoRoot is the path used to derive the default cache directory
	// (<RepoRoot>/.git/clarity). Required even when Flag or Env wins so
	// the default is always available.
	RepoRoot string
}

// ResolveCacheDir picks the active cache directory from s. The flag wins
// over the env var wins over the default — the same precedence every CLI
// tool uses for "per-invocation > persistent > built-in".
func ResolveCacheDir(s CacheDirSources) string {
	if s.Flag != "" {
		return s.Flag
	}
	if s.Env != "" {
		return s.Env
	}
	return filepath.Join(s.RepoRoot, ".git", "clarity")
}
