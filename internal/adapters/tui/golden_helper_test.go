package tui_test

import "github.com/ezcdlabs/clarity/internal/core"

// goldenView is the fixture the golden layout tests render. Deliberately
// varied: a wide distribution, a tight one, a sparse week, an empty week, and
// counts that differ enough for the bars to be distinguishable. Carries a repo
// name and resolved statuses so the header is the one a user actually sees.
func goldenView() core.View {
	return core.View{
		Snapshot: core.Snapshot{RepoName: "app"},
		Header:   core.HeaderStatus{CI: "passed", Deploy: "passed"},
		Flows: []core.FlowView{flowWith("deploy",
			weekOf(2026, 40, 18, 1, 1.5, 2, 2.5, 3, 4, 5, 7, 9, 12),
			weekOf(2026, 39, 24, 1, 1.2, 1.4, 1.6, 1.8, 2, 2.2, 2.6),
			weekOf(2026, 38, 3, 4, 6, 8),
			weekOf(2026, 37, 0),
			weekOf(2026, 36, 9, 2, 3, 4, 6, 8, 11),
		)},
	}
}
