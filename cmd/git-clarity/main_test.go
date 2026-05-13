package main

import (
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
