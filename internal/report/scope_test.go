package report

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestValidateScope covers the candidacy grammar's own rules. It shares the
// target format with deploy reporting — same append-only ref, same permanence
// — but differs in one way that matters: the target is required. "affected"
// with nothing to be affected *of* says nothing.
func TestValidateScope(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		wantErr string
	}{
		{name: "a plain target", target: "ios"},
		{name: "a scoped name", target: "@acme/api"},
		{name: "no target at all", target: "", wantErr: "target is required"},
		{name: "a blank target", target: "   ", wantErr: "whitespace"},
		{name: "an embedded newline", target: "ios\nevil", wantErr: "whitespace"},
		{name: "a shell metacharacter", target: "ios$(id)", wantErr: "characters"},
		{name: "absurdly long", target: strings.Repeat("a", 200), wantErr: "too long"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateScope(c.target)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
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

// The echoed recovery command has to reproduce a candidacy record as exactly
// as it reproduces an event — same reason, same pinned --sha and --at. A
// re-run next week that recorded today's fact at next week's timestamp would
// corrupt the lead time it exists to protect.
func TestScopeCommandLine_Reproduces(t *testing.T) {
	at := time.Unix(1744120134, 0).UTC()

	affected := ScopeCommandLine(ScopeOptions{SHA: "abc123", Time: at, Target: "ios", Affected: true})
	if !strings.HasSuffix(affected, "affected ios") {
		t.Errorf("affected command line is %q", affected)
	}
	if !strings.Contains(affected, "--sha abc123") || !strings.Contains(affected, "--at 2025-04-08T") {
		t.Errorf("command line does not pin sha and time: %q", affected)
	}

	unaffected := ScopeCommandLine(ScopeOptions{SHA: "abc123", Time: at, Target: "ios"})
	if !strings.HasSuffix(unaffected, "unaffected ios") {
		t.Errorf("unaffected command line is %q", unaffected)
	}
}

// A dropped candidacy record fails quietly — nothing renders as in-flight, the
// flow just keeps counting a commit it doesn't ship. So the failure has to
// carry its own recovery command, like a dropped event does.
func TestScopeFailureError_CarriesTheRecoveryCommand(t *testing.T) {
	opts := ScopeOptions{
		SHA: "9f9edc8612345678", Time: time.Unix(1744120134, 0).UTC(),
		Target: "android", Affected: false,
	}
	err := ScopeFailureError(opts, fmt.Errorf("push events ref: exit status 1"))

	msg := err.Error()
	if !strings.Contains(msg, ScopeCommandLine(opts)) {
		t.Errorf("failure does not repeat the command to re-run:\n%s", msg)
	}
	for _, want := range []string{"9f9edc86", "android", "lead time"} {
		if !strings.Contains(msg, want) {
			t.Errorf("failure does not mention %q:\n%s", want, msg)
		}
	}
	if !strings.Contains(msg, "exit status 1") {
		t.Errorf("failure hides its cause:\n%s", msg)
	}
}
