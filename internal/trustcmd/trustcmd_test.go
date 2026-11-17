package trustcmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/trust"
	"github.com/ezcdlabs/clarity/internal/trustcmd"
)

// execSourceRemote builds a remote whose trunk .clarity configures an exec
// source with the given argv lines.
func execSourceRemote(t *testing.T, argv ...string) *gittest.Remote {
	t.Helper()
	remote := gittest.NewRemote(t)
	writer := remote.NewClone(t)
	var b strings.Builder
	b.WriteString("[clarity]\n\tsource = exec\n[source]\n")
	for _, a := range argv {
		b.WriteString("\targ = " + a + "\n")
	}
	writer.WriteFile(".clarity", b.String())
	writer.CommitAll("clarity: exec source")
	writer.Push("main")
	return remote
}

func run(t *testing.T, repoPath, input string, interactive bool) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := trustcmd.Run(trustcmd.Options{
		RepoPath:    repoPath,
		In:          strings.NewReader(input),
		Out:         &out,
		Interactive: interactive,
	})
	return out.String(), err
}

// TestRun_TypedConfirmationRecordsTheApproval: the happy path — the exact
// command is shown, the user types the word, and this clone trusts this
// argv from then on.
func TestRun_TypedConfirmationRecordsTheApproval(t *testing.T) {
	remote := execSourceRemote(t, "./tools/helper", "--store=ch")
	clone := remote.NewClone(t)

	out, err := run(t, clone.Path, "trust\n", true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"./tools/helper", "--store=ch", "repository"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should show %q before asking:\n%s", want, out)
		}
	}
	if ok, _ := trust.IsTrusted(clone.Path, []string{"./tools/helper", "--store=ch"}); !ok {
		t.Fatal("the approval was not recorded")
	}
}

// TestRun_AnythingElseRecordsNothing: a prompt that accepts a stray
// keypress is a prompt that approves by accident.
func TestRun_AnythingElseRecordsNothing(t *testing.T) {
	remote := execSourceRemote(t, "./helper")
	clone := remote.NewClone(t)

	for _, input := range []string{"y\n", "yes\n", "TRUST\n", "\n", ""} {
		if _, err := run(t, clone.Path, input, true); err != nil {
			t.Fatalf("Run(%q): %v — declining is not an error", input, err)
		}
		if ok, _ := trust.IsTrusted(clone.Path, []string{"./helper"}); ok {
			t.Fatalf("input %q recorded an approval; only the exact word may", input)
		}
	}
}

// TestRun_NonInteractiveRefuses: in CI, hooks, pipes and agent sessions
// there is no human to answer, so there is nothing this command may do.
func TestRun_NonInteractiveRefuses(t *testing.T) {
	remote := execSourceRemote(t, "./helper")
	clone := remote.NewClone(t)

	_, err := run(t, clone.Path, "trust\n", false)
	if err == nil {
		t.Fatal("a non-interactive trust succeeded")
	}
	if !strings.Contains(err.Error(), "human") && !strings.Contains(err.Error(), "interactive") {
		t.Errorf("the refusal should say why: %v", err)
	}
	if ok, _ := trust.IsTrusted(clone.Path, []string{"./helper"}); ok {
		t.Fatal("a non-interactive run recorded an approval")
	}
}

// TestRun_NothingToTrust: a repo with no exec source has no command to
// approve, and saying so is the whole job.
func TestRun_NothingToTrust(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	out, err := run(t, clone.Path, "", true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "nothing to trust") {
		t.Errorf("output should say there is nothing to trust:\n%s", out)
	}
}

// TestRun_AlreadyTrustedSaysSoWithoutPrompting: re-running on an approved
// command must not re-ask — no input is consumed at all.
func TestRun_AlreadyTrustedSaysSoWithoutPrompting(t *testing.T) {
	remote := execSourceRemote(t, "./helper")
	clone := remote.NewClone(t)
	if _, err := run(t, clone.Path, "trust\n", true); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	out, err := run(t, clone.Path, "", true) // no input available
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if !strings.Contains(out, "already") {
		t.Errorf("output should say the command is already trusted:\n%s", out)
	}
}

// TestRun_ChangedCommandShowsOldAndNew: an edited .clarity means the old
// approval no longer applies; the prompt must show what it changed from so
// the human is deciding about the change, not re-reading from scratch.
func TestRun_ChangedCommandShowsOldAndNew(t *testing.T) {
	remote := execSourceRemote(t, "./helper", "--v1")
	clone := remote.NewClone(t)
	if _, err := run(t, clone.Path, "trust\n", true); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// The shared config moves on.
	writer := remote.NewClone(t)
	writer.WriteFile(".clarity", "[clarity]\n\tsource = exec\n[source]\n\targ = ./helper\n\targ = --v2\n")
	writer.CommitAll("clarity: new helper flag")
	writer.Push("main")
	clone.Fetch()

	out, err := run(t, clone.Path, "trust\n", true)
	if err != nil {
		t.Fatalf("Run after change: %v", err)
	}
	for _, want := range []string{"--v1", "--v2", "changed"} {
		if !strings.Contains(out, want) {
			t.Errorf("the changed-command prompt should show %q:\n%s", want, out)
		}
	}
	if ok, _ := trust.IsTrusted(clone.Path, []string{"./helper", "--v2"}); !ok {
		t.Fatal("the new command was not recorded after confirmation")
	}
	if ok, _ := trust.IsTrusted(clone.Path, []string{"./helper", "--v1"}); ok {
		t.Fatal("the old approval survived — recording must replace it")
	}
}
