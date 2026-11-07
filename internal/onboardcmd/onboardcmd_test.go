package onboardcmd_test

import (
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/onboardcmd"
)

// TestPrompt_IsSelfContained: the printed prompt is everything an agent
// needs — the event grammar, the refs, the batch backfill shape, the
// verification step — with zero web fetches. It is versioned with the
// binary precisely so it always describes the format this build reads.
func TestPrompt_IsSelfContained(t *testing.T) {
	out := onboardcmd.Prompt(onboardcmd.Facts{
		RemoteURL:          "git@github.com:acme/widgets.git",
		Trunk:              "master",
		SetUp:              false,
		HasGitHubWorkflows: true,
	})

	for _, want := range []string{
		// repo facts the agent should not have to rediscover
		"git@github.com:acme/widgets.git",
		"master",
		// the mechanism
		"refs/clarity/events",
		"refs/clarity/config",
		"git clarity init",
		// the full report grammar, exactly as this build parses it
		"git clarity report",
		"started", "passed", "failed", "skipped",
		"deploy",
		// backfill: batch form and the JSONL field names
		"--batch",
		"\"sha\"", "\"at\"", "RFC3339",
		// GitHub Actions path (these facts said workflows exist)
		"ezcdlabs/clarity@",
		"contents: write",
		// verification
		"git clarity --plain",
		// honesty constraint for the agent
		"Do not invent history",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

// TestPrompt_AlreadySetUpSkipsInit: an agent pointed at a configured repo
// must not be told to init again — the prompt says setup is done and moves
// straight to instrumentation.
func TestPrompt_AlreadySetUpSkipsInit(t *testing.T) {
	out := onboardcmd.Prompt(onboardcmd.Facts{RemoteURL: "x", Trunk: "main", SetUp: true})
	if !strings.Contains(out, "already set up") {
		t.Error("prompt should state setup is already done")
	}
}

// TestGatherFacts reads the real repository: remote, detected trunk, and
// the setup marker.
func TestGatherFacts(t *testing.T) {
	remote := gittest.NewRemoteOnBranch(t, "master")
	clone := remote.NewClone(t)

	f := onboardcmd.GatherFacts(clone.Path)
	if f.Trunk != "master" {
		t.Errorf("Trunk = %q, want master", f.Trunk)
	}
	if f.RemoteURL == "" {
		t.Error("RemoteURL is empty")
	}
	if f.SetUp {
		t.Error("SetUp = true on a fresh repo")
	}

	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", []byte("[clarity]\n\tsource = events-ref\n"), "clarity: set up"); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if f := onboardcmd.GatherFacts(clone.Path); !f.SetUp {
		t.Error("SetUp = false after the config ref was written")
	}
}
