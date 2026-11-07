// Package onboardcmd implements `git clarity onboard`: it prints a
// self-contained prompt that hands clarity's setup and backfill to an AI
// agent.
//
// The prompt lives in the binary rather than on a website for three
// reasons: it is versioned with the binary, so it always describes the
// event format this exact build reads instead of drifting independently;
// it works offline and behind firewalls; and the binary can tailor it with
// facts it gathers on the spot — the remote, the detected trunk, whether
// the repo is already set up — that a hosted document would make the agent
// rediscover.
package onboardcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gitenv"
	"github.com/ezcdlabs/clarity/internal/initcmd"
)

// Facts are what the binary knows about this repository, stamped into the
// prompt so the agent starts informed.
type Facts struct {
	RemoteURL          string // credential-redacted
	Trunk              string // detected; "" when nothing resolved
	SetUp              bool   // refs/clarity/config exists
	HasGitHubWorkflows bool   // .github/workflows is present
}

// GatherFacts inspects the repository. Everything is best-effort — a fact
// that cannot be gathered becomes its zero value and the prompt degrades
// to asking the agent to find out — except the config ref, which is
// fetched first so "set up" reflects the remote's truth when reachable.
func GatherFacts(repoPath string) Facts {
	f := Facts{
		RemoteURL: remoteURL(repoPath),
		Trunk:     initcmd.DetectTrunk(repoPath),
	}
	_ = clarityrefs.FetchConfigRef(repoPath, "origin")
	if _, ok, err := clarityrefs.ReadConfigFile(repoPath); err == nil && ok {
		f.SetUp = true
	}
	if info, err := os.Stat(filepath.Join(repoPath, ".github", "workflows")); err == nil && info.IsDir() {
		f.HasGitHubWorkflows = true
	}
	return f
}

// Prompt renders the agent prompt. One contiguous block on stdout, ready
// to paste into Claude Code or any other agent.
func Prompt(f Facts) string {
	var b strings.Builder

	b.WriteString("Set up clarity (git-clarity) on this repository and backfill its history.\n\n")
	b.WriteString("clarity is a git-native CD dashboard: it shows the trunk's commits flowing\n")
	b.WriteString("through CI and deploys, from data stored entirely in git refs — events in\n")
	b.WriteString("refs/clarity/events, config in refs/clarity/config, both pushed to origin\n")
	b.WriteString("like any other ref. Pushing them therefore needs write access to the repo.\n\n")

	b.WriteString("Facts gathered by `git clarity onboard` (trust these, don't rediscover them):\n")
	fmt.Fprintf(&b, "- remote: %s\n", orUnknown(f.RemoteURL))
	if f.Trunk != "" {
		fmt.Fprintf(&b, "- trunk: %s (detected via origin/HEAD, then main, then master)\n", f.Trunk)
	} else {
		b.WriteString("- trunk: not detected — find out which branch is the trunk, and run\n  `git remote set-head origin --auto` so origin/HEAD records it\n")
	}
	fmt.Fprintf(&b, "- clarity already set up (refs/clarity/config exists): %s\n", yesNo(f.SetUp))
	fmt.Fprintf(&b, "- GitHub Actions workflows present: %s\n\n", yesNo(f.HasGitHubWorkflows))

	b.WriteString("Do the following:\n\n")
	if f.SetUp {
		b.WriteString("1. Setup is already done (refs/clarity/config exists) — skip init.\n\n")
	} else {
		b.WriteString("1. Run `git clarity init`. It writes refs/clarity/config with defaults and\n")
		b.WriteString("   pushes it; that ref marks the repo as onboarded. Idempotent.\n\n")
	}

	b.WriteString("2. Instrument the pipeline so runs on the trunk report events:\n")
	b.WriteString("   - stages are exactly `ci` and `deploy`; statuses are `started`, `passed`,\n")
	b.WriteString("     `failed` or `skipped`.\n")
	b.WriteString("   - from any pipeline step: `git clarity report ci started`,\n")
	b.WriteString("     `git clarity report ci passed` (or `failed`), and for deploys\n")
	b.WriteString("     `git clarity report deploy passed [<target>]` — <target> names the\n")
	b.WriteString("     deployable in a repo that ships several from one trunk; omit it for a\n")
	b.WriteString("     single deploy flow. CI never takes a target.\n")
	if f.HasGitHubWorkflows {
		b.WriteString("   - this repo uses GitHub Actions, so prefer the ready-made action:\n")
		b.WriteString("       - uses: ezcdlabs/clarity@<latest release tag>\n")
		b.WriteString("         with: { stage: ci, status: passed }\n")
		b.WriteString("     The job needs `permissions: contents: write` (reporting pushes a ref;\n")
		b.WriteString("     the default GITHUB_TOKEN then suffices). Report `started` at the top\n")
		b.WriteString("     of the workflow and `passed`/`failed` at the end.\n")
	}
	b.WriteString("\n")

	b.WriteString("3. Backfill history so the dashboard is useful immediately. Walk the CI\n")
	b.WriteString("   provider's recorded runs for the trunk (on GitHub: `gh run list` /\n")
	b.WriteString("   `gh api`) and emit one JSON line per event, then pipe them through the\n")
	b.WriteString("   batch form — it makes a single commit and push for the whole backfill:\n\n")
	b.WriteString("     git clarity report --batch < events.jsonl\n\n")
	b.WriteString("   Each line: {\"stage\":\"ci\",\"status\":\"passed\",\"sha\":\"<commit sha>\",\n")
	b.WriteString("   \"at\":\"<RFC3339 timestamp>\"} — deploy lines may add \"target\".\n")
	b.WriteString("   Do not invent history: use only runs the provider actually recorded,\n")
	b.WriteString("   with their real commit SHAs and completion times.\n\n")

	b.WriteString("4. Verify with `git clarity --plain`: commits should now sit under their\n")
	b.WriteString("   CI/deploy dividers instead of all waiting \"before CI Passed\".\n")

	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func orUnknown(s string) string {
	if s == "" {
		return "(no origin remote found)"
	}
	return s
}

// remoteURL returns origin's URL with credentials redacted — the prompt is
// destined for an agent's context window, which is no place for a token
// that happens to live in a remote URL.
func remoteURL(repoPath string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return gitenv.RedactURL(strings.TrimSpace(string(out)))
}
