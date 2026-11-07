package initcmd_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/initcmd"
)

// TestRun_WritesConfigRefAndPushes: init is the act of writing
// refs/clarity/config — after it, the remote carries the ref, its file
// parses as the defaults, and the user was told what happened.
func TestRun_WritesConfigRefAndPushes(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	var out bytes.Buffer

	if err := initcmd.Run(initcmd.Options{RepoPath: clone.Path, Out: &out}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	cmd := exec.Command("git", "rev-parse", "--verify", clarityrefs.ConfigRef)
	cmd.Dir = remote.Path
	if err := cmd.Run(); err != nil {
		t.Fatalf("remote has no %s after init: %v", clarityrefs.ConfigRef, err)
	}

	data, ok, err := clarityrefs.ReadConfigFile(clone.Path)
	if err != nil || !ok {
		// Run deletes its local working ref after pushing; fetch it back.
		if err := clarityrefs.FetchConfigRef(clone.Path, "origin"); err != nil {
			t.Fatalf("fetch after init: %v", err)
		}
		data, ok, err = clarityrefs.ReadConfigFile(clone.Path)
	}
	if err != nil || !ok {
		t.Fatalf("no readable config after init: ok=%v err=%v", ok, err)
	}
	cfg, err := config.ParseClarityConfig(data)
	if err != nil {
		t.Fatalf("the config init wrote does not parse: %v\n%s", err, data)
	}
	if len(cfg.Deploys()) != 0 {
		t.Errorf("init declared deploys: %+v", cfg.Deploys())
	}
	if !strings.Contains(out.String(), "refs/clarity/config") {
		t.Errorf("output should name the ref it wrote:\n%s", out.String())
	}
}

// TestRun_AlreadySetUp_LeavesExistingConfigAlone: re-running init on a
// configured repo must not clobber choices someone already made — it
// reports the repo is set up and touches nothing.
func TestRun_AlreadySetUp_LeavesExistingConfigAlone(t *testing.T) {
	remote := gittest.NewRemote(t)
	first := remote.NewClone(t)
	second := remote.NewClone(t)

	custom := []byte("[clarity]\n\tsource = events-ref\n\tleadTime = reported\n")
	if err := clarityrefs.WriteConfigFile(first.Path, "origin", custom, "clarity: custom"); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	before := refSHA(t, remote.Path)

	var out bytes.Buffer
	if err := initcmd.Run(initcmd.Options{RepoPath: second.Path, Out: &out}); err != nil {
		t.Fatalf("Run on a set-up repo: %v", err)
	}
	if after := refSHA(t, remote.Path); after != before {
		t.Fatalf("init moved the config ref on an already-set-up repo: %s -> %s", before, after)
	}
	if !strings.Contains(out.String(), "already") {
		t.Errorf("output should say the repo is already set up:\n%s", out.String())
	}
}

func refSHA(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", clarityrefs.ConfigRef)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}
