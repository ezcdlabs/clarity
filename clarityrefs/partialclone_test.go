package clarityrefs_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// unreachableRemote is a remote that fails immediately and without a network
// round trip: port 1 on loopback refuses instantly, where a bogus hostname
// would depend on how the machine's DNS behaves.
const unreachableHost = "127.0.0.1:1"
const unreachableRemote = "http://" + unreachableHost + "/nope.git"

// TestWriteEvent_UnderBloblessPartialClone is the regression test for a report
// failing on any runner whose checkout makes a partial clone.
//
// Observed as `read events ref: object not found` against a repo whose events
// ref demonstrably existed on the remote. A blobless clone sets
// remote.origin.promisor, so clarity's own `git fetch` of the events ref
// brings the commit and its trees but leaves the event JSON blobs on the
// server for lazy retrieval. go-git reads the object store directly and has
// no promisor support, so the first blob read fails — and because the ref
// does exist locally by then, it fails as a missing *object* rather than a
// missing ref, which is what made the error so misleading.
//
// The event already on the remote is what makes this bite: a first-ever
// report has no blobs to read back and would pass regardless.
func TestWriteEvent_UnderBloblessPartialClone(t *testing.T) {
	remote := gittest.NewRemote(t)

	seed := remote.NewClone(t)
	first := clarityrefs.Event{Stage: "ci", Status: "started", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(seed.Path, "origin", fakeSHA, first); err != nil {
		t.Fatalf("seeding the remote events ref failed: %v", err)
	}

	partial := remote.NewBloblessClone(t)
	second := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120200, 0)}
	if err := clarityrefs.WriteEvent(partial.Path, "origin", fakeSHA, second); err != nil {
		t.Fatalf("WriteEvent from a blobless partial clone failed: %v", err)
	}

	// Both events must survive: the new one written, the pre-existing one
	// carried forward rather than dropped by a rebuild from an empty tree.
	verify := remote.NewClone(t)
	if err := fetchEventsRef(verify.Path); err != nil {
		t.Fatalf("fetch for verification failed: %v", err)
	}
	events, err := clarityrefs.ReadEvents(verify.Path, fakeSHA)
	if err != nil {
		t.Fatalf("ReadEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected both events on the remote, got %d: %+v", len(events), events)
	}
	if events[0].Status != "started" || events[1].Status != "passed" {
		t.Errorf("expected started then passed, got %q then %q", events[0].Status, events[1].Status)
	}
}

// TestReadAllEvents_UnderBloblessPartialClone covers the read side, which the
// TUI and `--plain` use. It broke the same way but surfaced even worse: the
// watcher swallowed the error and retried until the deadline, so the user saw
// only "context deadline exceeded" with no hint of the cause.
func TestReadAllEvents_UnderBloblessPartialClone(t *testing.T) {
	remote := gittest.NewRemote(t)

	seed := remote.NewClone(t)
	ev := clarityrefs.Event{Stage: "deploy", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(seed.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("seeding the remote events ref failed: %v", err)
	}

	partial := remote.NewBloblessClone(t)
	if err := clarityrefs.FetchEventsRef(partial.Path, "origin"); err != nil {
		t.Fatalf("FetchEventsRef into a blobless partial clone failed: %v", err)
	}
	all, err := clarityrefs.ReadAllEvents(partial.Path)
	if err != nil {
		t.Fatalf("ReadAllEvents in a blobless partial clone failed: %v", err)
	}
	if len(all[fakeSHA]) != 1 {
		t.Fatalf("expected 1 event for %s, got %d", fakeSHA, len(all[fakeSHA]))
	}
}

// TestFetchEventsRef_MissingRefIsNotAnError pins the genuine first-run case:
// a remote with no events ref yet is normal, not a failure, and must stay
// distinguishable from a fetch that could not reach the remote at all.
func TestFetchEventsRef_MissingRefIsNotAnError(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	if err := clarityrefs.FetchEventsRef(clone.Path, "origin"); err != nil {
		t.Fatalf("a remote with no events ref yet should not be an error, got: %v", err)
	}
}

// TestFetchEventsRef_UnreachableRemoteReportsTheRemote pins the other half of
// that distinction. "object not found" sent the reporter looking for a ref
// that was actually present; an unreachable remote has to say so, and say
// which remote, so the reader knows to look at credentials rather than at the
// ref.
func TestFetchEventsRef_UnreachableRemoteReportsTheRemote(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	gittest.SetRemoteURL(t, clone.Path, "origin", unreachableRemote)

	err := clarityrefs.FetchEventsRef(clone.Path, "origin")
	if err == nil {
		t.Fatal("fetching from an unreachable remote should be an error, got nil")
	}

	var fe *clarityrefs.FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected a *clarityrefs.FetchError, got %T: %v", err, err)
	}
	if fe.Remote != "origin" {
		t.Errorf("FetchError.Remote = %q, want %q", fe.Remote, "origin")
	}
	if !strings.Contains(fe.URL, unreachableHost) {
		t.Errorf("FetchError.URL = %q, want it to name the URL that was tried", fe.URL)
	}

	// Assert on clarity's own first line, not the whole message: git's output
	// is appended below it and happens to mention the host too, so a check
	// against the full string passes even when clarity names nothing itself.
	summary := strings.SplitN(err.Error(), "\n", 2)[0]
	if !strings.Contains(summary, "origin") {
		t.Errorf("error summary should name the remote it tried, got: %s", summary)
	}
	if !strings.Contains(summary, unreachableHost) {
		t.Errorf("error summary should name the remote URL it tried, got: %s", summary)
	}
	if !strings.Contains(summary, clarityrefs.EventsRef) {
		t.Errorf("error summary should name the ref it tried to fetch, got: %s", summary)
	}
	if strings.Contains(err.Error(), "object not found") {
		t.Errorf("a reachability failure must not be reported as a missing object, got: %s", err)
	}

	// git's own diagnosis is the actionable half — it is what names the
	// transport or auth problem — so it has to survive into the message
	// rather than being reduced to an exit status.
	if strings.TrimSpace(fe.Output) == "" {
		t.Error("FetchError.Output is empty; git's diagnosis was discarded")
	}
	if !strings.Contains(err.Error(), strings.TrimSpace(strings.SplitN(fe.Output, "\n", 2)[0])) {
		t.Errorf("error should include git's own output, got: %s", err)
	}
}

// TestWriteEvent_FallsBackWhenGitHasNoNoFilterFlag guards the fix against
// becoming a regression of its own.
//
// --no-filter arrived with partial clone in git 2.19. On anything older the
// flag is rejected outright, which would turn a fix for one kind of checkout
// into a total failure on old git — so the fetch retries without it. That is
// safe precisely because a git that cannot make a partial clone has no filter
// to opt out of.
//
// Driven through a shim on PATH that rejects the flag the way old git does
// and otherwise delegates to the real binary.
func TestWriteEvent_FallsBackWhenGitHasNoNoFilterFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell shim is POSIX-only")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}

	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	shimDir := t.TempDir()
	marker := filepath.Join(shimDir, "rejected")
	shim := "#!/bin/sh\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = \"--no-filter\" ]; then\n" +
		"    echo rejected >> " + marker + "\n" +
		"    echo \"error: unknown option \\`no-filter'\" >&2\n" +
		"    exit 129\n" +
		"  fi\n" +
		"done\n" +
		"exec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(shim), 0o755); err != nil {
		t.Fatalf("writing shim: %v", err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ev := clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, ev); err != nil {
		t.Fatalf("WriteEvent should fall back when git rejects --no-filter, got: %v", err)
	}

	// Without this the test would pass even if --no-filter were never sent,
	// which is the mutation it exists to catch.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the shim never saw --no-filter, so no fallback was exercised: %v", err)
	}

	verify := remote.NewClone(t)
	if err := fetchEventsRef(verify.Path); err != nil {
		t.Fatalf("fetch for verification failed: %v", err)
	}
	events, err := clarityrefs.ReadEvents(verify.Path, fakeSHA)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected the event to reach the remote, got %d events, err=%v", len(events), err)
	}
}

// TestWriteEvent_UnreachableRemoteFailsFast pins that the write path actually
// surfaces a fetch failure rather than swallowing it.
//
// It used to discard the fetch error and carry on. That was wrong twice over:
// the rebuild would start from an empty tree, so a push that somehow landed
// would drop every event already recorded, and a push rejected as
// non-fast-forward sent the retry loop back to a fetch that would fail
// identically, forever. The user-visible half is that the message blamed the
// push when the fetch was what broke.
func TestWriteEvent_UnreachableRemoteFailsFast(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	seed := clarityrefs.Event{Stage: "ci", Status: "started", Time: time.Unix(1744120134, 0)}
	if err := clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA, seed); err != nil {
		t.Fatalf("seeding failed: %v", err)
	}
	gittest.SetRemoteURL(t, clone.Path, "origin", unreachableRemote)

	done := make(chan error, 1)
	go func() {
		done <- clarityrefs.WriteEvent(clone.Path, "origin", fakeSHA,
			clarityrefs.Event{Stage: "ci", Status: "passed", Time: time.Unix(1744120200, 0)})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("writing to an unreachable remote should fail, got nil")
		}
		var fe *clarityrefs.FetchError
		if !errors.As(err, &fe) {
			t.Fatalf("expected the fetch failure to be reported as such, got %T: %v", err, err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("WriteEvent did not return: the retry loop is spinning on a fetch that cannot succeed")
	}
}
