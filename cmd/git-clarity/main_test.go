package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/cache"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/report"
)

func TestParseReportArgs_BarePositional(t *testing.T) {
	opts, err := parseReportArgs([]string{"ci", "passed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Stage != "ci" || opts.Status != "passed" {
		t.Errorf("stage/status: got %q/%q, want ci/passed", opts.Stage, opts.Status)
	}
	if opts.SHA != "" {
		t.Errorf("SHA should be empty without --sha, got %q", opts.SHA)
	}
	if !opts.Time.IsZero() {
		t.Errorf("Time should be zero without --at, got %v", opts.Time)
	}
}

func TestParseReportArgs_SHAFlag(t *testing.T) {
	const sha = "abc1234567890abc1234567890abc1234567890a"
	opts, err := parseReportArgs([]string{"--sha", sha, "ci", "passed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SHA != sha {
		t.Errorf("SHA: got %q, want %q", opts.SHA, sha)
	}
	if opts.Stage != "ci" || opts.Status != "passed" {
		t.Errorf("stage/status: got %q/%q, want ci/passed", opts.Stage, opts.Status)
	}
}

func TestParseReportArgs_AtFlag(t *testing.T) {
	const ts = "2024-04-08T15:48:54Z"
	opts, err := parseReportArgs([]string{"--at", ts, "deploy", "failed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, _ := time.Parse(time.RFC3339, ts)
	if !opts.Time.Equal(want) {
		t.Errorf("Time: got %v, want %v", opts.Time, want)
	}
	if opts.Stage != "deploy" || opts.Status != "failed" {
		t.Errorf("stage/status: got %q/%q, want deploy/failed", opts.Stage, opts.Status)
	}
}

func TestParseReportArgs_BothFlags(t *testing.T) {
	const sha = "abc1234567890abc1234567890abc1234567890a"
	const ts = "2024-04-08T15:48:54Z"
	opts, err := parseReportArgs([]string{"--sha", sha, "--at", ts, "ci", "passed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SHA != sha {
		t.Errorf("SHA: got %q, want %q", opts.SHA, sha)
	}
	want, _ := time.Parse(time.RFC3339, ts)
	if !opts.Time.Equal(want) {
		t.Errorf("Time: got %v, want %v", opts.Time, want)
	}
}

func TestParseReportArgs_EqualsForm(t *testing.T) {
	const sha = "abc1234567890abc1234567890abc1234567890a"
	const ts = "2024-04-08T15:48:54Z"
	opts, err := parseReportArgs([]string{"--sha=" + sha, "--at=" + ts, "ci", "passed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SHA != sha {
		t.Errorf("SHA: got %q, want %q", opts.SHA, sha)
	}
}

func TestParseReportArgs_RejectsBadTimestamp(t *testing.T) {
	_, err := parseReportArgs([]string{"--at", "not-a-time", "ci", "passed"})
	if err == nil {
		t.Fatal("expected error for invalid --at timestamp")
	}
}

func TestParseReportArgs_RejectsMissingPositional(t *testing.T) {
	for _, tc := range [][]string{
		{},
		{"ci"},
		// A third positional is now the deploy target, so the arity error
		// starts at four. Whether a target is legal for this stage is a
		// question for validation, which can explain itself.
		{"ci", "passed", "ios", "extra"},
		{"--sha", "abc"},
	} {
		if _, err := parseReportArgs(tc); err == nil {
			t.Errorf("expected error for args %v", tc)
		}
	}
}

// TestRunReport_RejectsATargetOnCI is the end-to-end half: the CLI must refuse
// before writing anything, and before echoing a command that would re-run the
// same mistake. The message has to teach, because someone reaching for
// `ci passed ios` is reaching for something the tool deliberately cannot do.
func TestRunReport_RejectsATargetOnCI(t *testing.T) {
	var out bytes.Buffer
	called := false
	write := func(report.Options) (string, error) {
		called = true
		return "", nil
	}

	err := runReportTo(&out, []string{"ci", "passed", "ios"}, write)
	if err == nil {
		t.Fatal("expected an error reporting a target on ci")
	}
	if called {
		t.Error("an event was written despite the invalid invocation")
	}
	if out.Len() != 0 {
		t.Errorf("echoed a command for an invocation it refuses: %q", out.String())
	}
	for _, want := range []string{"ci takes no target", "integrated"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%s", want, err)
		}
	}
}

func TestParseReportArgs_RejectsUnknownFlag(t *testing.T) {
	_, err := parseReportArgs([]string{"--bogus", "x", "ci", "passed"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestParseBatchLine_HappyPath(t *testing.T) {
	const line = `{"sha":"abc1234567890abc1234567890abc1234567890a","at":"2024-04-08T15:48:54Z","stage":"ci","status":"passed"}`
	ev, err := parseBatchLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.SHA != "abc1234567890abc1234567890abc1234567890a" {
		t.Errorf("SHA: got %q", ev.SHA)
	}
	want, _ := time.Parse(time.RFC3339, "2024-04-08T15:48:54Z")
	if !ev.Time.Equal(want) {
		t.Errorf("Time: got %v, want %v", ev.Time, want)
	}
	if ev.Stage != "ci" || ev.Status != "passed" {
		t.Errorf("stage/status: got %q/%q", ev.Stage, ev.Status)
	}
}

func TestParseBatchLine_RejectsBadJSON(t *testing.T) {
	if _, err := parseBatchLine("not json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseBatchLine_RejectsBadTimestamp(t *testing.T) {
	const line = `{"sha":"abc","at":"yesterday","stage":"ci","status":"passed"}`
	if _, err := parseBatchLine(line); err == nil {
		t.Fatal("expected error for invalid 'at' timestamp")
	}
}

func TestReadBatchEvents_SkipsBlankLines(t *testing.T) {
	input := strings.Join([]string{
		`{"sha":"a","at":"2024-04-08T15:48:54Z","stage":"ci","status":"started"}`,
		``,
		`   `,
		`{"sha":"b","at":"2024-04-08T15:49:54Z","stage":"ci","status":"passed"}`,
	}, "\n")
	events, err := readBatchEvents(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events (blank lines skipped), got %d", len(events))
	}
}

func TestReadBatchEvents_ReportsLineNumberOnError(t *testing.T) {
	input := strings.Join([]string{
		`{"sha":"a","at":"2024-04-08T15:48:54Z","stage":"ci","status":"started"}`,
		`bogus`,
	}, "\n")
	_, err := readBatchEvents(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error on malformed line 2")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should mention line 2, got: %v", err)
	}
}

func TestParseRootArgs_Defaults(t *testing.T) {
	opts, err := parseRootArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.plain {
		t.Errorf("plain should default to false")
	}
	if opts.showSHAs {
		t.Errorf("showSHAs should default to false")
	}
	if opts.limit != 100 {
		t.Errorf("limit should default to 100, got %d", opts.limit)
	}
}

func TestParseRootArgs_PlainFlag(t *testing.T) {
	opts, err := parseRootArgs([]string{"--plain"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.plain {
		t.Errorf("--plain should set plain=true")
	}
}

func TestParseRootArgs_ShowShas(t *testing.T) {
	opts, err := parseRootArgs([]string{"--show-shas"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.showSHAs {
		t.Errorf("--show-shas should set showSHAs=true")
	}
}

func TestParseRootArgs_LimitOverride(t *testing.T) {
	opts, err := parseRootArgs([]string{"--limit", "25"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.limit != 25 {
		t.Errorf("--limit 25 should set limit=25, got %d", opts.limit)
	}
}

func TestParseRootArgs_LimitZeroIsUnlimited(t *testing.T) {
	opts, err := parseRootArgs([]string{"--limit", "0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 0 is the documented sentinel for "no cap" — main translates this into a
	// huge ceiling when calling BuildSnapshot.
	if opts.limit != 0 {
		t.Errorf("--limit 0 should be preserved as the unlimited sentinel, got %d", opts.limit)
	}
}

func TestParseRootArgs_AllFlagsTogether(t *testing.T) {
	opts, err := parseRootArgs([]string{"--plain", "--show-shas", "--limit", "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.plain || !opts.showSHAs || opts.limit != 10 {
		t.Errorf("flags should combine, got %+v", opts)
	}
}

func TestParseRootArgs_RejectsUnknownFlag(t *testing.T) {
	if _, err := parseRootArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestIsBatchInvocation(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"--batch"}, true},
		{[]string{"ci", "passed"}, false},
		{[]string{"--sha", "abc", "ci", "passed"}, false},
		{[]string{"--batch", "extra"}, true},
		{nil, false},
	} {
		if got := isBatchInvocation(tc.args); got != tc.want {
			t.Errorf("isBatchInvocation(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

// TestRunReport_EchoesFullyExplicitCommand checks the first thing a report
// invocation prints is the fully-resolved form of itself. A CI step that is
// killed or times out never reaches its error path, so the recovery command
// has to be in the log before the write is attempted, not only after it fails.
func TestRunReport_EchoesFullyExplicitCommand(t *testing.T) {
	var out bytes.Buffer
	var got report.Options
	write := func(o report.Options) (string, error) {
		got = o
		return o.SHA, nil
	}

	args := []string{"--sha", "abc1234", "--at", "2026-08-04T12:30:05Z", "deploy", "passed"}
	if err := runReportTo(&out, args, write); err != nil {
		t.Fatalf("runReportTo: %v", err)
	}

	want := "running: git clarity report --sha abc1234 --at 2026-08-04T12:30:05Z deploy passed"
	if !strings.Contains(out.String(), want) {
		t.Errorf("expected the echoed command %q in:\n%s", want, out.String())
	}
	// The echo has to describe the event actually written, or it is worse
	// than useless as a retry instruction.
	if got.SHA != "abc1234" {
		t.Errorf("wrote SHA %q, want the echoed abc1234", got.SHA)
	}
}

// TestRunReport_EchoesResolvedValues checks the echo is the *resolved* form
// even when the caller supplied neither flag — echoing back a bare
// `report deploy passed` would give the user nothing to retry with, since
// re-running it later resolves a different SHA and timestamp.
func TestRunReport_EchoesResolvedValues(t *testing.T) {
	t.Setenv("GITHUB_SHA", "fedcba9876543210fedcba9876543210fedcba98")
	var out bytes.Buffer
	write := func(o report.Options) (string, error) { return o.SHA, nil }

	if err := runReportTo(&out, []string{"deploy", "passed"}, write); err != nil {
		t.Fatalf("runReportTo: %v", err)
	}

	line := out.String()
	if !strings.Contains(line, "--sha fedcba9876543210fedcba9876543210fedcba98") {
		t.Errorf("expected the resolved SHA in the echo:\n%s", line)
	}
	if !strings.Contains(line, "--at 20") {
		t.Errorf("expected a resolved --at timestamp in the echo:\n%s", line)
	}
}

// TestRunReport_FailureCarriesRetryCommand is the reported symptom: a deploy
// report that fails leaves the commit spinning in the TUI, with nothing in
// the log saying how to put the event back.
func TestRunReport_FailureCarriesRetryCommand(t *testing.T) {
	var out bytes.Buffer
	write := func(report.Options) (string, error) {
		return "", errors.New("push events ref: exit status 1")
	}

	args := []string{"--sha", "abc1234", "--at", "2026-08-04T12:30:05Z", "deploy", "passed"}
	err := runReportTo(&out, args, write)
	if err == nil {
		t.Fatal("expected the failure to surface")
	}
	msg := err.Error()

	if !strings.Contains(msg, "failed to report deploy passed") {
		t.Errorf("expected the failure to name the stage and status:\n%s", msg)
	}
	if !strings.Contains(msg, "git clarity report --sha abc1234 --at 2026-08-04T12:30:05Z deploy passed") {
		t.Errorf("expected the retry command in the failure:\n%s", msg)
	}
}

// TestRunReport_SuccessIsUnchanged guards the confirmation line the echo sits
// above — the new output adds to it rather than replacing it.
func TestRunReport_SuccessIsUnchanged(t *testing.T) {
	var out bytes.Buffer
	write := func(report.Options) (string, error) {
		return "9f9edc8673b331befd2adda3eadb62effde0fbe9", nil
	}

	args := []string{"--sha", "9f9edc8673b331befd2adda3eadb62effde0fbe9", "ci", "passed"}
	if err := runReportTo(&out, args, write); err != nil {
		t.Fatalf("runReportTo: %v", err)
	}
	if !strings.Contains(out.String(), "wrote event: 9f9edc86 ci passed") {
		t.Errorf("expected the confirmation line:\n%s", out.String())
	}
}

// stubSource emits one snapshot and closes, so the lens wiring can be
// exercised without git or the GitHub API.
type stubSource struct{ snap core.Snapshot }

func (s *stubSource) Watch(ctx context.Context) <-chan core.Snapshot {
	ch := make(chan core.Snapshot, 1)
	ch <- s.snap
	close(ch)
	return ch
}

func targetedSnapshot() core.Snapshot {
	return core.Snapshot{
		RepoName: "clarity",
		Commits: []core.CommitView{
			{SHA: "a", Author: "alice", Subject: "x", Time: time.Unix(1000, 0),
				Events: []clarityrefs.Event{{Stage: "deploy", Status: "passed", Time: time.Unix(1100, 0)}}},
			{SHA: "b", Author: "bob", Subject: "y", Time: time.Unix(800, 0),
				Events: []clarityrefs.Event{{Stage: "deploy", Status: "passed", Time: time.Unix(900, 0), Target: "ios"}}},
		},
	}
}

func configWithFlows(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	body := `{"clarity": {"deploys": [{"name": "web", "targets": ["", "web"]}, {"name": "ios", "targets": ["ios"]}]}}`
	if err := os.WriteFile(filepath.Join(dir, ".ezcd.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write .ezcd.json: %v", err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func firstView(t *testing.T, views <-chan core.View) core.View {
	t.Helper()
	select {
	case v, ok := <-views:
		if !ok {
			t.Fatal("lens closed without emitting a view")
		}
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a view")
		return core.View{}
	}
}

// TestLensWiring_PassesDeclaredFlows guards the three lines where the whole
// feature can silently do nothing. Both render paths build their lens here,
// and a lens built without the declared flows falls back to discovery — which
// looks plausible (two flows appear!) while ignoring the config entirely.
//
// The declared names are the tell: discovery would call the untargeted flow
// "deploy", never "web".
func TestLensWiring_PassesDeclaredFlows(t *testing.T) {
	cfg := configWithFlows(t)

	t.Run("plain path", func(t *testing.T) {
		view := firstView(t, lensFor(cfg, &stubSource{snap: targetedSnapshot()}).Views(t.Context()))
		assertDeclaredFlows(t, view)
	})

	t.Run("tui path", func(t *testing.T) {
		cf := cache.New(filepath.Join(t.TempDir(), "snapshot-cache.json.gz"))
		lens := cachedLensFor(cfg, &stubSource{snap: targetedSnapshot()}, cf)
		// The cached lens can emit a stale frame first; the configured flows
		// must be present on every frame it produces, not just the fresh one.
		views := lens.Views(t.Context())
		assertDeclaredFlows(t, firstView(t, views))
	})
}

func assertDeclaredFlows(t *testing.T, view core.View) {
	t.Helper()
	if len(view.Flows) != 2 {
		t.Fatalf("want 2 declared flows, got %d", len(view.Flows))
	}
	if view.Flows[0].Name != "web" || view.Flows[1].Name != "ios" {
		t.Errorf("flows are %q/%q, want web/ios — the declared config did not reach the lens",
			view.Flows[0].Name, view.Flows[1].Name)
	}
}

// TestParseReportArgs_Target covers the third positional argument. Arity is
// the whole check: a target is only meaningful for deploy, and `ci passed ios`
// has to fail in the pipeline that wrote it rather than quietly recording an
// event nobody can explain later.
func TestParseReportArgs_Target(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTarget string
		wantErr    string
	}{
		{name: "deploy with a target", args: []string{"deploy", "passed", "ios"}, wantTarget: "ios"},
		{name: "deploy without a target", args: []string{"deploy", "passed"}, wantTarget: ""},
		{name: "ci without a target", args: []string{"ci", "passed"}, wantTarget: ""},
		{
			name: "flags still parse before the positionals",
			args: []string{"--sha", "abc", "deploy", "passed", "android"}, wantTarget: "android",
		},
		{
			name:    "a fourth positional is rejected",
			args:    []string{"deploy", "passed", "ios", "extra"},
			wantErr: "usage",
		},
		{
			name:    "no positionals at all",
			args:    []string{},
			wantErr: "usage",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts, err := parseReportArgs(c.args)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("got error %v, want one mentioning %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts.Target != c.wantTarget {
				t.Errorf("Target = %q, want %q", opts.Target, c.wantTarget)
			}
		})
	}
}

// The usage line must show the target, or the only way to discover the
// argument is to read the docs.
func TestParseReportArgs_UsageMentionsTheTarget(t *testing.T) {
	_, err := parseReportArgs(nil)
	if err == nil {
		t.Fatal("expected a usage error")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("usage does not mention the target argument: %v", err)
	}
}

// Backfill has to be able to reproduce a targeted deploy, or migrating a
// monorepo's history would flatten every flow into the untargeted one.
func TestParseBatchLine_Target(t *testing.T) {
	ev, err := parseBatchLine(`{"sha":"abc","at":"2024-04-08T15:48:54Z","stage":"deploy","status":"passed","target":"ios"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Target != "ios" {
		t.Errorf("Target = %q, want ios", ev.Target)
	}

	bare, err := parseBatchLine(`{"sha":"abc","at":"2024-04-08T15:48:54Z","stage":"deploy","status":"passed"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bare.Target != "" {
		t.Errorf("untargeted line gained a target: %q", bare.Target)
	}
}

// An explicitly empty target is the shell case a pipeline actually hits:
// `git clarity report deploy passed "$TARGET"` with TARGET unset. Recording an
// untargeted deploy there, with a zero exit, puts the event in the wrong flow
// and says nothing about it.
func TestParseReportArgs_RejectsAnExplicitlyEmptyTarget(t *testing.T) {
	if _, err := parseReportArgs([]string{"deploy", "passed", ""}); err == nil {
		t.Fatal("an empty target argument was accepted")
	} else if !strings.Contains(err.Error(), "omit the argument") {
		t.Errorf("error does not say what to do instead: %v", err)
	}

	// Omitting it entirely is still the way to report the untargeted deploy.
	opts, err := parseReportArgs([]string{"deploy", "passed"})
	if err != nil || opts.Target != "" {
		t.Errorf("omitting the target should report untargeted, got %+v (err %v)", opts, err)
	}
}

// A batch error has to name the line someone can go and look at. Blank lines
// are skipped during parsing, so a slice index drifts from the input line by
// however many were skipped — and a generated backfill is exactly where that
// difference costs real time.
func TestReadBatchEvents_CarriesLineNumbers(t *testing.T) {
	in := "\n\n" + `{"sha":"abc","at":"2024-04-08T15:48:54Z","stage":"deploy","status":"passed"}` + "\n"
	events, err := readBatchEvents(strings.NewReader(in))
	if err != nil {
		t.Fatalf("readBatchEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Line != 3 {
		t.Errorf("event records line %d, want 3 — two blank lines were skipped before it", events[0].Line)
	}
}

// TestCheckDeclaredTarget is where declaring flows pays off: a typo in a
// pipeline fails that pipeline, rather than surfacing days later as a flow
// nobody deploys to and nobody can delete.
func TestCheckDeclaredTarget(t *testing.T) {
	declared := `{"clarity": {"deploys": [{"name": "web", "targets": ["", "web"]}, "ios"]}}`

	cases := []struct {
		name    string
		config  string
		target  string
		wantErr string
	}{
		{name: "a declared target", config: declared, target: "ios"},
		{name: "a declared alias", config: declared, target: "web"},
		{name: "no target at all", config: declared, target: ""},
		{name: "nothing declared keeps the open vocabulary", config: `{"clarity": {}}`, target: "anything"},
		{name: "no config file at all", config: "", target: "anything"},
		{
			name: "an undeclared target", config: declared, target: "android",
			wantErr: "not declared",
		},
		{
			name: "a typo", config: declared, target: "isos",
			wantErr: "isos",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if c.config != "" {
				if err := os.WriteFile(filepath.Join(dir, ".ezcd.json"), []byte(c.config), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := checkDeclaredTarget(dir, c.target)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not mention %q", err, c.wantErr)
			}
			// The alternatives have to be listed, or the user is guessing.
			for _, want := range []string{"web", "ios"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error does not list declared flow %q: %v", want, err)
				}
			}
		})
	}
}

// TestRootFlags_ReachTheRenderers guards the wiring between a parsed flag and
// the code that acts on it. A flag can parse correctly, be threaded into
// rootOptions, and still never reach a renderer — at which point it is inert
// and looks like a working feature from every angle except using it.
func TestRootFlags_ReachTheRenderers(t *testing.T) {
	opts, err := parseRootArgs([]string{"--deploy", "ios", "--show-shas"})
	if err != nil {
		t.Fatalf("parseRootArgs: %v", err)
	}
	if opts.deploy != "ios" {
		t.Fatalf("--deploy parsed as %q", opts.deploy)
	}

	if got := plainRendererFor(opts).Opts().Flow; got != "ios" {
		t.Errorf("plain renderer built with flow %q, want ios", got)
	}
	if got := plainRendererFor(opts).Opts().ShowSHAs; !got {
		t.Error("plain renderer did not receive --show-shas")
	}
	if got := tuiRendererFor(opts).Flow(); got != "ios" {
		t.Errorf("TUI renderer built with flow %q, want ios", got)
	}
}

// TestIsScopeInvocation pins the dispatch between the two grammars. It has to
// look past the flags for the verb, or `report --sha X affected ios` takes the
// stage path and fails with a confusing stage error.
func TestIsScopeInvocation(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{args: []string{"affected", "ios"}, want: true},
		{args: []string{"unaffected", "ios"}, want: true},
		{args: []string{"--sha", "abc", "affected", "ios"}, want: true},
		{args: []string{"--sha", "abc", "--at", "2024-04-08T15:48:54Z", "unaffected", "web"}, want: true},
		{args: []string{"deploy", "passed"}, want: false},
		{args: []string{"ci", "passed"}, want: false},
		{args: []string{"--sha", "abc", "deploy", "passed", "ios"}, want: false},
		// A deploy target named "affected" must not be mistaken for the verb:
		// the verb is the first positional, the target is the third.
		{args: []string{"deploy", "passed", "affected"}, want: false},
		{args: []string{}, want: false},
	}

	for _, c := range cases {
		if got := isScopeInvocation(c.args); got != c.want {
			t.Errorf("isScopeInvocation(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestParseScopeArgs(t *testing.T) {
	opts, err := parseScopeArgs([]string{"--sha", "abc", "affected", "ios"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.Affected || opts.Target != "ios" || opts.SHA != "abc" {
		t.Errorf("parsed %+v", opts)
	}

	un, err := parseScopeArgs([]string{"unaffected", "android"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if un.Affected || un.Target != "android" {
		t.Errorf("parsed %+v", un)
	}

	for _, bad := range [][]string{{"affected"}, {"affected", "ios", "extra"}, {"affected", ""}} {
		if _, err := parseScopeArgs(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}

// TestDispatchReport_RoutesToTheRightGrammar checks that the routing predicate
// is actually consulted. A correct predicate that nothing calls leaves the
// candidacy grammar unreachable, and `report affected ios` then fails with a
// stage error that explains nothing.
func TestDispatchReport_RoutesToTheRightGrammar(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantScope bool
	}{
		{name: "candidacy", args: []string{"affected", "ios"}, wantScope: true},
		{name: "candidacy behind flags", args: []string{"--sha", "abc", "unaffected", "ios"}, wantScope: true},
		{name: "a stage event", args: []string{"deploy", "passed", "ios"}, wantScope: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotEvent, gotScope bool
			err := dispatchReport(&bytes.Buffer{}, c.args,
				func(report.Options) (string, error) {
					gotEvent = true
					return "abcdef1234", nil
				},
				func(report.ScopeOptions) (string, error) {
					gotScope = true
					return "abcdef1234", nil
				},
			)
			if err != nil {
				t.Fatalf("dispatchReport: %v", err)
			}
			if gotScope != c.wantScope || gotEvent == c.wantScope {
				t.Errorf("routed to scope=%v event=%v, want scope=%v", gotScope, gotEvent, c.wantScope)
			}
		})
	}
}
