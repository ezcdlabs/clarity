package report

import (
	"strings"
	"testing"
	"time"
)

// TestValidateTarget is the classifier for the third positional argument.
//
// The rule that matters is that CI may never carry one. A target that can pass
// CI on its own is not integrated with the rest of the repo, and clarity's
// whole claim is that "is main green?" has exactly one answer per commit. The
// error therefore has to teach that, not just state a rule.
func TestValidateTarget(t *testing.T) {
	cases := []struct {
		name    string
		stage   string
		status  string
		target  string
		wantErr string // substring; empty means the call must succeed
	}{
		{name: "deploy with a target", stage: "deploy", status: "passed", target: "ios"},
		{name: "deploy without a target", stage: "deploy", status: "passed"},
		{name: "ci without a target", stage: "ci", status: "passed"},
		{
			name:  "ci with a target is rejected",
			stage: "ci", status: "passed", target: "ios",
			wantErr: "ci takes no target",
		},
		{
			name:  "ci with a target is rejected whatever the status",
			stage: "ci", status: "started", target: "web",
			wantErr: "ci takes no target",
		},
		{
			name:  "a blank target is not a target",
			stage: "ci", status: "passed", target: "   ",
			wantErr: "whitespace",
		},
		{
			name:  "a blank target on deploy is also rejected",
			stage: "deploy", status: "passed", target: "  ",
			wantErr: "whitespace",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateReport(c.stage, c.status, c.target)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error mentioning %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not mention %q", err, c.wantErr)
			}
		})
	}
}

// TestValidateTarget_ErrorExplainsWhy pins the reasoning in the message. A
// bare "invalid argument" would leave someone adding `ci passed ios` to a
// pipeline with no idea why clarity refuses, and the refusal is the single
// most opinionated thing the tool does.
func TestValidateTarget_ErrorExplainsWhy(t *testing.T) {
	err := validateReport("ci", "passed", "ios")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"ios", "integrated", "deploy"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %q:\n%s", want, msg)
		}
	}
}

// TestCommandLine_ReproducesTheTarget guards the recovery path. A dropped
// report leaves a commit showing as in-flight forever, and the echoed command
// is what someone re-runs to fix it — so it has to reproduce the event
// exactly, target included, or the retry lands in the wrong flow.
func TestCommandLine_ReproducesTheTarget(t *testing.T) {
	at := time.Unix(1744120134, 0).UTC()

	withTarget := CommandLine(Options{SHA: "abc123", Time: at, Stage: "deploy", Status: "passed", Target: "ios"})
	if !strings.HasSuffix(withTarget, "deploy passed ios") {
		t.Errorf("command line does not reproduce the target: %q", withTarget)
	}

	// An untargeted report must not grow a trailing argument, or re-running
	// the echoed command would report to a flow literally named "".
	without := CommandLine(Options{SHA: "abc123", Time: at, Stage: "deploy", Status: "passed"})
	if !strings.HasSuffix(without, "deploy passed") {
		t.Errorf("untargeted command line gained an argument: %q", without)
	}
}

// TestValidateTarget_Format constrains what may be written. Events are
// append-only and content-addressed, so a target written once is a flow label
// forever — there is no edit and no delete. That makes the write the only
// place this can be caught, and makes being strict here cheap compared to the
// alternative.
func TestValidateTarget_Format(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		wantErr string
	}{
		{name: "a plain name", target: "ios"},
		{name: "digits and dashes", target: "ios-15"},
		{name: "dots and slashes", target: "apps/web"},
		{name: "scoped package style", target: "@acme/api"},
		{name: "underscores", target: "edge_worker"},
		{name: "mixed case is fine", target: "iOS"},

		{
			name: "surrounding whitespace", target: " ios ",
			// Would be a separate flow from `ios` forever, and the declarer
			// trims while Claims compares exactly, so it could never be claimed.
			wantErr: "whitespace",
		},
		{name: "an inner space", target: "my target", wantErr: "whitespace"},
		{name: "a newline", target: "ios\nevil", wantErr: "whitespace"},
		{name: "a tab", target: "ios\tx", wantErr: "whitespace"},
		{
			name: "an ANSI escape", target: "\x1b[31mred\x1b[0m",
			wantErr: "characters",
		},
		{name: "a control character", target: "ios\x07", wantErr: "characters"},
		{name: "a shell metacharacter", target: "ios$(id)", wantErr: "characters"},
		{name: "a quote", target: `ios"x`, wantErr: "characters"},
		{name: "leading dash looks like a flag", target: "--sha", wantErr: "characters"},
		{name: "absurdly long", target: strings.Repeat("a", 200), wantErr: "too long"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateReport("deploy", "passed", c.target)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", c.target, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %q to be rejected", c.target)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not mention %q", err, c.wantErr)
			}
		})
	}
}

// Whatever is accepted has to survive a round trip through the echoed recovery
// command, which is a shell command line. That is the documented purpose of
// the echo, so the accepted set and the shell-safe set must be the same set.
func TestValidateTarget_AcceptedTargetsAreShellWordSafe(t *testing.T) {
	for _, target := range []string{"ios", "ios-15", "apps/web", "@acme/api", "edge_worker", "a.b+c:d"} {
		if err := validateReport("deploy", "passed", target); err != nil {
			t.Fatalf("%q should be accepted: %v", target, err)
		}
		line := CommandLine(Options{SHA: "abc", Time: time.Unix(0, 0), Stage: "deploy", Status: "passed", Target: target})
		if fields := strings.Fields(line); fields[len(fields)-1] != target {
			t.Errorf("%q does not survive the echoed command as one word: %q", target, line)
		}
	}
}
