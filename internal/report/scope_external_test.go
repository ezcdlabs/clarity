package report_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/report"
)

// TestRunScope_WritesCandidacyToTheRef is the assertion the feature rests on.
// Everything upstream can validate correctly and the record can still never
// reach the ref, in which case the lead time it exists to correct stays wrong
// and nothing says so.
func TestRunScope_WritesCandidacyToTheRef(t *testing.T) {
	clearEnv(t)
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile("a.txt", "x")
	clone.CommitAll("commit one")
	clone.Push("main")

	head := headSHA(t, clone.Path)

	if _, err := report.RunScope(report.ScopeOptions{
		RepoPath: clone.Path, Remote: "origin", Target: "android", Affected: false,
	}); err != nil {
		t.Fatalf("RunScope: %v", err)
	}
	if _, err := report.RunScope(report.ScopeOptions{
		RepoPath: clone.Path, Remote: "origin", Target: "ios", Affected: true,
	}); err != nil {
		t.Fatalf("RunScope: %v", err)
	}

	fetchEventsRef(t, clone.Path)
	got, err := clarityrefs.ReadAllScope(clone.Path)
	if err != nil {
		t.Fatalf("ReadAllScope: %v", err)
	}

	byTarget := map[string]bool{}
	for _, s := range got[head] {
		byTarget[s.Target] = s.Affected
	}
	if affected, ok := byTarget["android"]; !ok || affected {
		t.Errorf("android candidacy did not round-trip as unaffected: %+v", got[head])
	}
	if affected, ok := byTarget["ios"]; !ok || !affected {
		t.Errorf("ios candidacy did not round-trip as affected: %+v", got[head])
	}

	// Candidacy must not appear as a pipeline event — that is the whole
	// reason it lives in its own tree.
	events, err := clarityrefs.ReadEvents(clone.Path, head)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("candidacy surfaced as %d pipeline events: %+v", len(events), events)
	}
}
