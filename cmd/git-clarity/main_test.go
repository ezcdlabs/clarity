package main

import (
	"strings"
	"testing"
	"time"
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
