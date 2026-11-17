package config

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
)

func TestParseClarityConfig(t *testing.T) {
	cases := map[string]struct {
		in      string
		want    Config
		wantErr string // substring the error must carry; "" means no error
	}{
		"empty file is all defaults": {
			in:   "",
			want: Config{Branch: DefaultBranch},
		},
		"source events-ref and a lead time mode": {
			in: "[clarity]\n\tsource = events-ref\n\tleadTime = reported\n",
			want: Config{
				Branch:   DefaultBranch,
				LeadTime: core.LeadReported,
			},
		},
		"deploy subsections become declared flows in order": {
			in: "[deploy \"web\"]\n\ttarget = web-eu\n\ttarget = web-us\n[deploy \"mobile\"]\n",
			want: Config{
				Branch: DefaultBranch,
				DeclaredDeploys: []core.Flow{
					{Name: "web", Targets: []string{"web-eu", "web-us"}},
					{Name: "mobile", Targets: []string{"mobile"}},
				},
			},
		},
		"unknown source is this build's cue to refuse": {
			in:      "[clarity]\n\tsource = github\n",
			wantErr: "events-ref",
		},
		"source exec takes its argv from the source section": {
			in: "[clarity]\n\tsource = exec\n[source]\n\targ = bazel\n\targ = run\n\targ = //tools/clarity-source\n",
			want: Config{
				Branch:     DefaultBranch,
				SourceExec: &ExecSource{Argv: []string{"bazel", "run", "//tools/clarity-source"}, Timeout: DefaultExecTimeout},
			},
		},
		"source exec honours a timeout": {
			in: "[clarity]\n\tsource = exec\n[source]\n\targ = ./helper\n\ttimeout = 30s",
			want: Config{
				Branch:     DefaultBranch,
				SourceExec: &ExecSource{Argv: []string{"./helper"}, Timeout: 30 * time.Second},
			},
		},
		"arguments are an argv, never a shell line": {
			// Metacharacters are data. If any layer ever interprets these,
			// the argv read back here would differ. The quoting is git-config
			// dialect, not shell: unquoted `;` and `#` start a comment, so a
			// value carrying one is written quoted — and that quote is the
			// only processing the value ever gets.
			in: "[clarity]\n\tsource = exec\n[source]\n\targ = ./helper\n\targ = \"$(touch pwned); rm -rf *\"\n",
			want: Config{
				Branch:     DefaultBranch,
				SourceExec: &ExecSource{Argv: []string{"./helper", "$(touch pwned); rm -rf *"}, Timeout: DefaultExecTimeout},
			},
		},
		"source exec with no argv is an error": {
			in:      "[clarity]\n\tsource = exec\n",
			wantErr: "arg",
		},
		"a bad exec timeout is an error": {
			in:      "[clarity]\n\tsource = exec\n[source]\n\targ = ./helper\n\ttimeout = soon\n",
			wantErr: "timeout",
		},
		"a source section without source exec is ignored like any unknown section": {
			in:   "[source]\n\targ = ./helper\n",
			want: Config{Branch: DefaultBranch},
		},
		"unknown lead time mode is an error": {
			in:      "[clarity]\n\tleadTime = sideways\n",
			wantErr: "lead time",
		},
		"a target claimed by two flows is an error": {
			in:      "[deploy \"web\"]\n\ttarget = prod\n[deploy \"api\"]\n\ttarget = prod\n",
			wantErr: "claimed by both",
		},
		"unknown sections and keys are ignored": {
			in:   "[clarity]\n\tfuturekey = whatever\n[somesection]\n\tx = 1\n",
			want: Config{Branch: DefaultBranch},
		},
		"malformed syntax is an error": {
			in:      "[clarity\nsource=",
			wantErr: "parse",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseClarityConfig([]byte(tc.in))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (config %+v)", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.LeadTime != tc.want.LeadTime {
				t.Errorf("LeadTime = %q, want %q", got.LeadTime, tc.want.LeadTime)
			}
			if len(got.DeclaredDeploys) != len(tc.want.DeclaredDeploys) {
				t.Fatalf("DeclaredDeploys = %+v, want %+v", got.DeclaredDeploys, tc.want.DeclaredDeploys)
			}
			for i, f := range got.DeclaredDeploys {
				w := tc.want.DeclaredDeploys[i]
				if f.Name != w.Name || strings.Join(f.Targets, ",") != strings.Join(w.Targets, ",") {
					t.Errorf("flow[%d] = %+v, want %+v", i, f, w)
				}
			}
			if !reflect.DeepEqual(got.SourceExec, tc.want.SourceExec) {
				t.Errorf("SourceExec = %+v, want %+v", got.SourceExec, tc.want.SourceExec)
			}
		})
	}
}

// The file init writes must parse back to the defaults it meant — the
// encoder and parser are two halves of one format and must not drift.
func TestDefaultClarityConfigFileRoundTrips(t *testing.T) {
	data := DefaultClarityConfigFile()
	cfg, err := ParseClarityConfig(data)
	if err != nil {
		t.Fatalf("the default config file does not parse: %v\n%s", err, data)
	}
	if cfg.LeadTimeMode() != core.DefaultLeadTimeMode {
		t.Errorf("default file changed the lead time mode: %q", cfg.LeadTimeMode())
	}
	if len(cfg.Deploys()) != 0 {
		t.Errorf("default file declared deploys: %+v", cfg.Deploys())
	}
	if !strings.Contains(string(data), "source = events-ref") {
		t.Errorf("default file should pin the source explicitly:\n%s", data)
	}
}
