package configload_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/configload"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestLoad_TrunkFileIsTheConfigOfRecord: with .clarity on the trunk, its
// values apply and both legacy formats — the config ref and .ezcd.json —
// are ignored outright rather than merged.
func TestLoad_TrunkFileIsTheConfigOfRecord(t *testing.T) {
	remote := gittest.NewRemote(t)
	writer := remote.NewClone(t)
	writer.WriteFile(".clarity", "[clarity]\n\tleadTime = reported\n")
	writer.CommitAll("add .clarity")
	writer.Push("main")

	reader := remote.NewClone(t)
	reader.WriteFile(".ezcd.json", `{"clarity": {"leadTime": "all"}}`)
	refCfg := []byte("[clarity]\n\tleadTime = all\n")
	if err := clarityrefs.WriteConfigFile(reader.Path, "origin", refCfg, "clarity: set up"); err != nil {
		t.Fatalf("seed ref config: %v", err)
	}

	cfg, err := configload.Load(reader.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SetUp {
		t.Error("SetUp = false with .clarity on the trunk")
	}
	if cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("LeadTimeMode = %q, want %q from .clarity on the trunk", cfg.LeadTimeMode(), core.LeadReported)
	}
	if cfg.Notice != "" {
		t.Errorf("Notice = %q on a fully migrated repo, want none", cfg.Notice)
	}
}

// TestLoad_ConfigRefIsHonouredWithAMigrationNotice: a repo set up before
// .clarity existed keeps working from refs/clarity/config, and the Notice
// says where the config should move.
func TestLoad_ConfigRefIsHonouredWithAMigrationNotice(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	refCfg := []byte("[clarity]\n\tleadTime = reported\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", refCfg, "clarity: set up"); err != nil {
		t.Fatalf("seed ref config: %v", err)
	}

	cfg, err := configload.Load(clone.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SetUp {
		t.Error("SetUp = false with the legacy config ref present")
	}
	if cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("LeadTimeMode = %q, want %q from the ref", cfg.LeadTimeMode(), core.LeadReported)
	}
	if cfg.Notice == "" || !contains(cfg.Notice, ".clarity") {
		t.Errorf("Notice = %q, want a migration notice naming .clarity", cfg.Notice)
	}
}

// TestLoad_WorkingTreeClarityCountsAsSetUpButItsValuesDoNot: a .clarity
// that exists only in the working tree suppresses the welcome pane — the
// commit is presumably in flight — but its values must not apply. Checking
// out a branch with a different .clarity must never change what clarity
// runs; only the trunk's copy speaks.
func TestLoad_WorkingTreeClarityCountsAsSetUpButItsValuesDoNot(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile(".clarity", "[clarity]\n\tleadTime = all\n")

	cfg, err := configload.Load(clone.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SetUp {
		t.Error("SetUp = false with a working-tree .clarity pending its commit")
	}
	if cfg.LeadTimeMode() != core.DefaultLeadTimeMode {
		t.Errorf("LeadTimeMode = %q from the working tree, want the %q default — working-tree values must not apply",
			cfg.LeadTimeMode(), core.DefaultLeadTimeMode)
	}
	if cfg.Notice == "" || !contains(cfg.Notice, "commit") {
		t.Errorf("Notice = %q, want a note that the file still needs committing", cfg.Notice)
	}
}

// TestLoad_LegacyJSONIsStillTheLastFallback: only .ezcd.json → its values
// load, but the repo does not count as set up.
func TestLoad_LegacyJSONIsStillTheLastFallback(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile(".ezcd.json", `{"clarity": {"leadTime": "reported"}}`)

	cfg, err := configload.Load(clone.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SetUp {
		t.Error("SetUp = true from a legacy .ezcd.json alone")
	}
	if !cfg.LegacyFile {
		t.Error("LegacyFile = false with .ezcd.json present")
	}
	if cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("LeadTimeMode = %q, want %q from the legacy file", cfg.LeadTimeMode(), core.LeadReported)
	}
}

// TestLoad_DetectsTheTrunk: nothing configured anywhere — the trunk still
// resolves from the repository itself, and the lookup follows it (a master
// repo must read .clarity from master, not from a hardcoded main).
func TestLoad_DetectsTheTrunk(t *testing.T) {
	remote := gittest.NewRemoteOnBranch(t, "master")
	writer := remote.NewClone(t)
	writer.WriteFile(".clarity", "[clarity]\n\tleadTime = reported\n")
	writer.CommitAll("add .clarity")
	writer.Push("master")

	reader := remote.NewClone(t)
	cfg, err := configload.Load(reader.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Branch != "master" {
		t.Errorf("Branch = %q, want detected master", cfg.Branch)
	}
	if !cfg.SetUp || cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("SetUp=%v LeadTimeMode=%q — .clarity on a master trunk was not read", cfg.SetUp, cfg.LeadTimeMode())
	}
}

// TestStored_Precedence: the raw-bytes read configcmd builds on — the trunk
// file when it exists, the legacy ref otherwise, and nothing when neither.
func TestStored_Precedence(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	if _, origin, err := configload.Stored(clone.Path, "main"); err != nil || origin != configload.OriginNone {
		t.Errorf("fresh repo: origin=%v err=%v, want OriginNone", origin, err)
	}

	refCfg := []byte("[clarity]\n\tleadTime = all\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", refCfg, "clarity: set up"); err != nil {
		t.Fatalf("seed ref config: %v", err)
	}
	if data, origin, err := configload.Stored(clone.Path, "main"); err != nil || origin != configload.OriginConfigRef || string(data) != string(refCfg) {
		t.Errorf("ref only: origin=%v err=%v data=%q, want the ref's bytes", origin, err, data)
	}

	clone.WriteFile(config.ClarityFileName, "[clarity]\n\tleadTime = reported\n")
	clone.CommitAll("add .clarity")
	clone.Push("main")
	if data, origin, err := configload.Stored(clone.Path, "main"); err != nil || origin != configload.OriginTrunkFile || string(data) != "[clarity]\n\tleadTime = reported\n" {
		t.Errorf("trunk file: origin=%v err=%v data=%q, want the trunk file's bytes", origin, err, data)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// TestLoad_FetchesTheTrunkWhenNoLocalRefHasIt: a CI job checked out from a
// tag has neither refs/remotes/origin/<trunk> nor refs/heads/<trunk>, and
// that job may be exactly the one reporting deploys — declared-target
// enforcement must not silently switch off there. When no local ref can
// answer for the trunk, the tip is fetched (best-effort, depth 1) before
// concluding the repo has no config.
func TestLoad_FetchesTheTrunkWhenNoLocalRefHasIt(t *testing.T) {
	remote := gittest.NewRemote(t)
	writer := remote.NewClone(t)
	writer.WriteFile(".clarity", "[clarity]\n\tleadTime = reported\n")
	writer.CommitAll("add .clarity")
	writer.Push("main")

	// A tag-checkout shape: detached HEAD, no trunk refs anywhere local.
	clone := remote.NewClone(t)
	for _, args := range [][]string{
		{"git", "remote", "set-head", "origin", "main"}, // origin/HEAD names the trunk, as it does after a real clone
		{"git", "checkout", "--detach", "HEAD"},
		{"git", "update-ref", "-d", "refs/heads/main"},
		{"git", "update-ref", "-d", "refs/remotes/origin/main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = clone.Path
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	cfg, err := configload.Load(clone.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SetUp || cfg.LeadTimeMode() != core.LeadReported {
		t.Errorf("SetUp=%v LeadTimeMode=%q — the trunk tip was not fetched when no local ref had it",
			cfg.SetUp, cfg.LeadTimeMode())
	}
}

// TestLoad_ExecSourceIsNeverHonouredFromTheLegacyRef: the ref is exactly
// where an attacker would put an executable command — unprotected,
// unreviewed, silently force-pushable — so a source command there is
// dropped with a warning, never run. Only .clarity on the trunk may name
// one.
func TestLoad_ExecSourceIsNeverHonouredFromTheLegacyRef(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	refCfg := []byte("[clarity]\n\tsource = exec\n[source]\n\targ = ./pwn\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", refCfg, "clarity: legacy"); err != nil {
		t.Fatalf("seed ref: %v", err)
	}

	cfg, err := configload.Load(clone.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SourceExec != nil {
		t.Fatalf("SourceExec = %+v from the legacy ref; it must be dropped", cfg.SourceExec)
	}
	if !strings.Contains(cfg.Notice, "source") {
		t.Errorf("Notice = %q, want a warning that the ref's source command was ignored", cfg.Notice)
	}
}

// TestLoad_ExecSourceFromTheTrunkIsParsed: the positive half — on the
// trunk the exec source loads with its argv intact.
func TestLoad_ExecSourceFromTheTrunkIsParsed(t *testing.T) {
	remote := gittest.NewRemote(t)
	writer := remote.NewClone(t)
	writer.WriteFile(".clarity", "[clarity]\n\tsource = exec\n[source]\n\targ = ./tools/helper\n\targ = --store=ch\n")
	writer.CommitAll("clarity: exec source")
	writer.Push("main")

	reader := remote.NewClone(t)
	cfg, err := configload.Load(reader.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SourceExec == nil || strings.Join(cfg.SourceExec.Argv, " ") != "./tools/helper --store=ch" {
		t.Errorf("SourceExec = %+v, want the trunk file's argv", cfg.SourceExec)
	}
}
