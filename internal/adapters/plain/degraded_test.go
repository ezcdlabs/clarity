package plain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/adapters/plain"
	"github.com/ezcdlabs/clarity/internal/core"
)

// TestRenderSnapshot_DegradedSourceIsNamed: piped output must say when the
// data is a last good read rather than a fresh one — and say why, because
// stderr from the helper is the only diagnostic anyone will get.
func TestRenderSnapshot_DegradedSourceIsNamed(t *testing.T) {
	v := core.DeriveView(core.Snapshot{RepoName: "app", Degraded: "helper exited 1: connection refused"}, core.LeadAll, nil)
	out := plain.RenderSnapshot("app", v, time.Unix(0, 0), plain.Options{})
	if !strings.Contains(out, "source degraded") || !strings.Contains(out, "connection refused") {
		t.Errorf("plain output does not name the degraded source:\n%s", out)
	}

	healthy := core.DeriveView(core.Snapshot{RepoName: "app"}, core.LeadAll, nil)
	if out := plain.RenderSnapshot("app", healthy, time.Unix(0, 0), plain.Options{}); strings.Contains(out, "degraded") {
		t.Errorf("healthy output mentions degradation:\n%s", out)
	}
}
