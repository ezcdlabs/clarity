// Package watch decides what a background check is worth telling someone about.
//
// The question is never "what is the state?" but "what changed?". A check runs
// every quarter of an hour whether or not anything happened, and a notification
// that fires on the state rather than the transition is one that goes off four
// times an hour for as long as a build stays broken — which is a notification
// the user turns off, taking the useful ones with it.
package watch

import (
	"fmt"

	"github.com/ezcdlabs/clarity/mobile/internal/registry"
)

// Change is one pipeline crossing between green and red.
type Change struct {
	// Stage names what moved, in the words a notification uses: "CI", or
	// "deploy to ios". Here rather than in each app, so two clients cannot
	// disagree about what to call a deploy target.
	Stage string
	// Broke distinguishes the two directions. Both are worth saying, and they
	// are not worth saying the same way.
	Broke bool
	From  string
	To    string
}

// Compare reports the crossings between a recorded verdict and a fresh one.
//
// before is nil for a repository nothing has read yet, and that is deliberately
// silent: there is nothing to have transitioned from, and announcing the state
// of a repository that was just added is wrong on its own terms — it did not
// break, it was always like that.
//
// The guard is also the only thing standing between a caller and a nil
// dereference. It is not what makes a first check quiet, though — the rule in
// crossing already does that, since an empty verdict is neither passed nor
// failed and so cannot be either end of a crossing.
func Compare(before *registry.Status, after registry.Status) []Change {
	if before == nil {
		return nil
	}

	var out []Change
	if c, ok := crossing("CI", before.CI, after.CI); ok {
		out = append(out, c)
	}

	// Per target, because a repo that ships web and ios from one trunk has two
	// pipelines — and "something went red" that does not say which is a
	// notification you have to open the app to understand.
	was := make(map[string]string, len(before.Flows))
	for _, f := range before.Flows {
		was[f.Name] = f.Deploy
	}
	for _, f := range after.Flows {
		previous, seen := was[f.Name]
		if !seen {
			// A target appearing for the first time has no previous verdict,
			// exactly as a new repository does not.
			continue
		}
		if c, ok := crossing(deployStage(f.Name), previous, f.Deploy); ok {
			out = append(out, c)
		}
	}
	return out
}

// deployStage names a target in the words a notification uses.
func deployStage(name string) string {
	if name == "" {
		return "deploy"
	}
	return fmt.Sprintf("deploy to %s", name)
}

// crossing reports whether two verdicts are a move between green and red.
//
// Only passed and failed count. Everything else — a build in flight, a stage
// nothing has reported — is not an answer, and treating the gap between a push
// and a green tick as a recovery would fire on every commit. It also means a
// pipeline that goes red, runs a fix and fails again is announced once: the
// second failure arrives from "started" rather than from "passed", and the
// reader has already been told it is broken.
func crossing(stage, before, after string) (Change, bool) {
	if before == after {
		return Change{}, false
	}
	switch {
	case before == "passed" && after == "failed":
		return Change{Stage: stage, Broke: true, From: before, To: after}, true
	case before == "failed" && after == "passed":
		return Change{Stage: stage, Broke: false, From: before, To: after}, true
	default:
		return Change{}, false
	}
}
