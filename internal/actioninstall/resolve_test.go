package actioninstall

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scriptPath locates scripts/action-install.sh relative to this file, so the
// test does not depend on the working directory `go test` chose.
func scriptPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "scripts", "action-install.sh")
}

// resolve runs the installer in resolve-only mode, which prints the version it
// would install and exits before touching the network.
func resolve(t *testing.T, env map[string]string) (version, source string) {
	t.Helper()

	cmd := exec.Command("bash", scriptPath(t))
	// A bare environment, so a GITHUB_* variable set by the CI running these
	// tests cannot leak in and decide the answer.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"CLARITY_RESOLVE_ONLY=1",
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("action-install.sh: %v\n%s", err, out)
	}
	for _, field := range strings.Fields(strings.TrimSpace(string(out))) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch k {
		case "version":
			version = v
		case "source":
			source = v
		}
	}
	if version == "" {
		t.Fatalf("no version in output: %q", out)
	}
	return version, source
}

// TestResolveVersion covers which version the composite action installs.
//
// The regression this exists for: `uses: ezcdlabs/clarity@v0.3.1` installed
// whatever had shipped most recently instead of v0.3.1, because the runner
// leaves GITHUB_ACTION_REF unset inside a composite action and the script had
// no other source for the ref. Nothing caught it — the workflow self-test
// invokes the action as `uses: ./` and the local script hardcoded an empty
// ref, so the pinned-tag branch was the one path never executed.
//
// A silently ignored pin is worse than a loud failure: builds drift with no
// diff to show for it, and pinning is normally a supply-chain control.
func TestResolveVersion(t *testing.T) {
	const unpacked = "/home/runner/work/_actions/ezcdlabs/clarity/"

	tests := []struct {
		name       string
		env        map[string]string
		wantVer    string
		wantSource string
	}{
		{
			name:       "a pinned tag is honoured, from the unpack path alone",
			env:        map[string]string{"GITHUB_ACTION_PATH": unpacked + "v0.3.1"},
			wantVer:    "v0.3.1",
			wantSource: "action-path",
		},
		{
			name: "an explicit version input beats the pinned tag",
			env: map[string]string{
				"INPUT_VERSION":      "v9.9.9",
				"GITHUB_ACTION_PATH": unpacked + "v0.3.1",
			},
			wantVer:    "v9.9.9",
			wantSource: "input",
		},
		{
			name: "GITHUB_ACTION_REF is preferred when the runner does set it",
			env: map[string]string{
				"GITHUB_ACTION_REF":  "v0.4.0",
				"GITHUB_ACTION_PATH": unpacked + "v0.3.1",
			},
			wantVer:    "v0.4.0",
			wantSource: "action-ref",
		},
		{
			name:       "a two-part tag is a valid pin",
			env:        map[string]string{"GITHUB_ACTION_PATH": unpacked + "v1.2"},
			wantVer:    "v1.2",
			wantSource: "action-path",
		},

		// Everything below means "track the newest release", which is what
		// these refs mean in practice. Treating any of them as a version
		// would send the installer after a release that does not exist.
		{
			name:       "a branch ref tracks latest",
			env:        map[string]string{"GITHUB_ACTION_PATH": unpacked + "main"},
			wantVer:    "latest",
			wantSource: "default",
		},
		{
			name:       "a commit sha tracks latest",
			env:        map[string]string{"GITHUB_ACTION_PATH": unpacked + "781bc347b1d4731c13a625a24af190aaf385880d"},
			wantVer:    "latest",
			wantSource: "default",
		},
		{
			name:       "a local uses:./ checkout tracks latest",
			env:        map[string]string{"GITHUB_ACTION_PATH": "/home/runner/work/clarity/clarity"},
			wantVer:    "latest",
			wantSource: "default",
		},
		{
			name:       "a pre-release tag is not a plain version tag",
			env:        map[string]string{"GITHUB_ACTION_PATH": unpacked + "v1.2.3-rc1"},
			wantVer:    "latest",
			wantSource: "default",
		},
		{
			name:       "nothing set at all",
			env:        map[string]string{},
			wantVer:    "latest",
			wantSource: "default",
		},
		{
			name:       "version: latest is passed through",
			env:        map[string]string{"INPUT_VERSION": "latest"},
			wantVer:    "latest",
			wantSource: "input",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotVer, gotSource := resolve(t, tc.env)
			if gotVer != tc.wantVer {
				t.Errorf("version = %q, want %q", gotVer, tc.wantVer)
			}
			if gotSource != tc.wantSource {
				t.Errorf("source = %q, want %q (version was %q)", gotSource, tc.wantSource, gotVer)
			}
		})
	}
}
