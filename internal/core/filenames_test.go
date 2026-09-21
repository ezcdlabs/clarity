package core_test

import (
	"os/exec"
	"strings"
	"testing"
)

// windowsReserved are the device names Windows refuses to use as a file name,
// with or without an extension. They date back to DOS and are still enforced
// by the Win32 API today.
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// TestNoWindowsReservedFilenames refuses a tracked file whose name Windows
// cannot create.
//
// internal/core/aux.go sat in the tree for four months and made the whole
// repository impossible to check out on Windows: `actions/checkout` failed
// with "error: invalid path 'internal/core/aux.go'" before running a single
// step. It went unnoticed because the only Windows job lives in the action
// workflow, which runs on a path filter, so its failures were not in front of
// anyone — and a developer on Windows would simply have been unable to clone.
//
// The check is trivial next to the cost of the mistake, and it runs on every
// push rather than only when the action changes.
func TestNoWindowsReservedFilenames(t *testing.T) {
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	for _, path := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if path == "" {
			continue
		}
		// Every segment matters, not just the file: a directory named "aux"
		// is equally impossible.
		for _, segment := range strings.Split(path, "/") {
			stem, _, _ := strings.Cut(segment, ".")
			if windowsReserved[strings.ToUpper(stem)] {
				t.Errorf("%s: %q is a reserved device name on Windows, so this "+
					"path cannot be checked out there", path, segment)
			}
		}
	}
}
