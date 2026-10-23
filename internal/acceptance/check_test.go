package acceptance_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/adapters/refsource"
	"github.com/ezcdlabs/clarity/internal/checkcmd"
	"github.com/ezcdlabs/clarity/internal/clock"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/report"
)

// `git clarity check` across the seams it actually has to cross: a real
// remote, a real events ref written by the real `report` path, and a working
// copy that has not seen either.
//
// The unit tests pin the classification. What they cannot pin is the thing
// most likely to be wrong, and the thing that would make the command
// dangerous rather than merely useless: *which* trunk it answers about. A
// check that read the local branch would say green in exactly the situation
// the command exists for — an agent whose clone predates the break — and
// would do it silently.

// checkIn runs the real pre-push path from inside repoPath: configure the
// fetch refspec, fetch the branch and the events ref, walk, classify. A fake
// clock keeps the Source's poll loop from ever ticking, so this is the single
// snapshot `check` takes and nothing more.
func checkIn(t *testing.T, repoPath string) checkcmd.Result {
	t.Helper()
	src, err := refsource.New(refsource.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Branch:   "main",
		Limit:    100,
		Clock:    clock.NewFake(),
	})
	if err != nil {
		t.Fatalf("refsource.New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return checkcmd.FromSource(ctx, src, "main", time.Now())
}

// reportCI writes a real CI event through the real report path, the way a
// pipeline would.
//
// The SHA is passed explicitly rather than left to resolve from HEAD, because
// report reads GITHUB_SHA from the environment first — and this suite's own CI
// sets it, which would silently attach every event in the test to the commit
// the suite is running against instead of the one it just made.
func reportCI(t *testing.T, repoPath, status string) {
	t.Helper()
	if _, err := report.Run(report.Options{
		RepoPath: repoPath,
		Remote:   "origin",
		Stage:    "ci",
		Status:   status,
		SHA:      headOf(t, repoPath),
	}); err != nil {
		t.Fatalf("report ci %s: %v", status, err)
	}
}

func headOf(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD in %s: %v", repoPath, err)
	}
	return strings.TrimSpace(string(out))
}

// pushCommit lands one commit on main from its own clone — one agent, one
// worktree, one push.
func pushCommit(t *testing.T, remote *gittest.Remote, file, subject string) *gittest.Clone {
	t.Helper()
	c := remote.NewClone(t)
	c.WriteFile(file, "package x\n")
	c.CommitAll(subject)
	c.Push("main")
	return c
}

// The case the command exists for: another agent broke CI, and this clone has
// not even fetched the commit that did it.
func TestCheck_RedOnTheRemoteIsRedHere(t *testing.T) {
	remote := gittest.NewRemote(t)

	// Cloned before the break landed, so local main is stale. This is the
	// agent about to push.
	pusher := remote.NewClone(t)

	breaker := pushCommit(t, remote, "broken.go", "feat: a commit that fails ci")
	reportCI(t, breaker.Path, "failed")

	res := checkIn(t, pusher.Path)

	if res.Outcome != checkcmd.Red {
		t.Fatalf("outcome = %v, want Red — check answered about the local branch, not the remote\nreport:\n%s",
			res.Outcome, res.Report)
	}
	if !strings.Contains(res.Report, "feat: a commit that fails ci") {
		t.Errorf("report does not name the breaking commit:\n%s", res.Report)
	}
	if !strings.Contains(res.Report, headOf(t, breaker.Path)[:7]) {
		t.Errorf("report does not carry the short SHA of the breaking commit:\n%s", res.Report)
	}
	if res.Outcome.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", res.Outcome.ExitCode())
	}
}

func TestCheck_GreenTrunkIsSilent(t *testing.T) {
	remote := gittest.NewRemote(t)
	pusher := remote.NewClone(t)

	good := pushCommit(t, remote, "good.go", "feat: a commit that passes ci")
	reportCI(t, good.Path, "passed")

	res := checkIn(t, pusher.Path)

	if res.Outcome != checkcmd.Green {
		t.Fatalf("outcome = %v, want Green\nreport:\n%s", res.Outcome, res.Report)
	}
	if res.Report != "" {
		t.Errorf("green printed a report, want silence:\n%s", res.Report)
	}
	if res.Outcome.ExitCode() != 0 {
		t.Errorf("exit code = %d, want 0", res.Outcome.ExitCode())
	}
}

// Once the break is fixed on top, everyone is unblocked without anyone
// telling check anything. This is the behaviour that makes the convention
// survivable: the thing that clears it is the ordinary act of fixing main.
func TestCheck_AFixOnTopClearsIt(t *testing.T) {
	remote := gittest.NewRemote(t)

	breaker := pushCommit(t, remote, "broken.go", "feat: breaks ci")
	reportCI(t, breaker.Path, "failed")

	pusher := remote.NewClone(t)
	if res := checkIn(t, pusher.Path); res.Outcome != checkcmd.Red {
		t.Fatalf("outcome before the fix = %v, want Red", res.Outcome)
	}

	fixer := pushCommit(t, remote, "fixed.go", "fix: unbreaks ci")
	reportCI(t, fixer.Path, "passed")

	if res := checkIn(t, pusher.Path); res.Outcome != checkcmd.Green {
		t.Errorf("outcome after the fix = %v, want Green\nreport:\n%s", res.Outcome, res.Report)
	}
}

// A repo that has never reported an event must not block pushes. Clarity adds
// its fetch refspec on first run and finds no such ref on the remote; a check
// that treated a missing events ref as anything other than "nobody has said"
// would make the command unusable in the repo it was just installed into.
func TestCheck_NeverReportedTrunkDoesNotBlock(t *testing.T) {
	remote := gittest.NewRemote(t)
	pusher := remote.NewClone(t)
	pushCommit(t, remote, "work.go", "feat: no ci in this repo at all")

	res := checkIn(t, pusher.Path)

	if res.Outcome != checkcmd.Green {
		t.Errorf("outcome = %v, want Green\nreport:\n%s", res.Outcome, res.Report)
	}
}

// The clone that broke CI is blocked too, and this is the case that decides
// whether the command works at all for the thing it was built for.
//
// Several agents in worktrees of one clone share a git identity and share the
// remote-tracking reflog, so any rule of the form "let the author of the break
// through" lets *every* agent in the fleet through — removing the speed bump
// precisely where nobody is watching. Whoever owns the fix pushes it by
// dropping the `&&`; check does not try to work out who that is.
func TestCheck_TheCloneThatBrokeItIsBlockedToo(t *testing.T) {
	remote := gittest.NewRemote(t)

	breaker := pushCommit(t, remote, "broken.go", "feat: breaks ci")
	reportCI(t, breaker.Path, "failed")

	res := checkIn(t, breaker.Path)

	if res.Outcome != checkcmd.Red {
		t.Fatalf("outcome in the breaking clone = %v, want Red — the speed bump is gone for the fleet that broke it\nreport:\n%s",
			res.Outcome, res.Report)
	}
	if res.Outcome.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", res.Outcome.ExitCode())
	}
}
