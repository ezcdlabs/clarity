package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

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
		{"ci", "passed", "extra"},
		{"--sha", "abc"},
	} {
		if _, err := parseReportArgs(tc); err == nil {
			t.Errorf("expected error for args %v", tc)
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
