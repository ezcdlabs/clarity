package config_test

import (
	"path/filepath"
	"testing"

	"github.com/ezcdlabs/clarity/internal/config"
)

// TestResolveCacheDir_Default falls back to <repoRoot>/.git/clarity when
// neither the flag nor the env is set. This matches the historical
// hard-coded path so existing users don't have to opt in to anything to
// keep their cache where it was.
func TestResolveCacheDir_Default(t *testing.T) {
	got := config.ResolveCacheDir(config.CacheDirSources{
		Flag: "", Env: "", RepoRoot: "/repos/clarity",
	})
	want := filepath.Join("/repos/clarity", ".git", "clarity")
	if got != want {
		t.Errorf("expected default %q, got %q", want, got)
	}
}

// TestResolveCacheDir_EnvOverridesDefault: the CLARITY_CACHE_DIR env var
// is the containerised-deployment path — point at a mounted volume so
// the gh-runs cache survives container restarts.
func TestResolveCacheDir_EnvOverridesDefault(t *testing.T) {
	got := config.ResolveCacheDir(config.CacheDirSources{
		Flag: "", Env: "/var/lib/clarity", RepoRoot: "/repos/clarity",
	})
	if got != "/var/lib/clarity" {
		t.Errorf("expected env value /var/lib/clarity, got %q", got)
	}
}

// TestResolveCacheDir_FlagBeatsEnv: per-invocation override wins over
// the persistent env setting — same precedence rule every CLI tool uses.
func TestResolveCacheDir_FlagBeatsEnv(t *testing.T) {
	got := config.ResolveCacheDir(config.CacheDirSources{
		Flag: "/tmp/c", Env: "/var/lib/clarity", RepoRoot: "/repos/clarity",
	})
	if got != "/tmp/c" {
		t.Errorf("expected flag value /tmp/c, got %q", got)
	}
}
