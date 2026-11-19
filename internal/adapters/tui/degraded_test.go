package tui

import (
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/core"
)

// TestHeader_DegradedSourceIsMarked: when the source reports itself
// failing, the header must say so — a dashboard quietly showing the last
// good data is exactly the silent-watcher failure the marker exists to
// prevent.
func TestHeader_DegradedSourceIsMarked(t *testing.T) {
	view := core.View{Snapshot: core.Snapshot{RepoName: "app", Degraded: "helper exited 1"}}
	line, _ := renderHeader(view, "", 100, nil, nil, "⠋")
	if !strings.Contains(line, "⚠") || !strings.Contains(line, "source") {
		t.Errorf("header does not mark the degraded source:\n%s", line)
	}

	healthy := core.View{Snapshot: core.Snapshot{RepoName: "app"}}
	if line, _ := renderHeader(healthy, "", 100, nil, nil, "⠋"); strings.Contains(line, "⚠") {
		t.Errorf("a healthy source shows a warning:\n%s", line)
	}
}
