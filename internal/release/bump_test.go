package release

import "testing"

// TestBumpFor documents how Conventional Commit messages map onto a semantic
// version bump. feat → minor, fix → patch, a `!` marker or a BREAKING CHANGE
// footer → major, and everything else (docs, refactor, chore, ci, test…) → no
// release. The overall bump is the highest implied by any commit in the set.
func TestBumpFor(t *testing.T) {
	cases := []struct {
		name     string
		messages []string
		want     Bump
	}{
		{"empty", nil, BumpNone},
		{"fix is patch", []string{"fix: correct off-by-one"}, BumpPatch},
		{"feat is minor", []string{"feat: add --json flag"}, BumpMinor},
		{"scoped fix is patch", []string{"fix(report): survive concurrent gc"}, BumpPatch},
		{"scoped feat is minor", []string{"feat(tui): live refresh"}, BumpMinor},
		{"bang marker is major", []string{"feat!: drop v1 ref format"}, BumpMajor},
		{"scoped bang is major", []string{"refactor(api)!: rename WriteEvent"}, BumpMajor},
		{"breaking footer is major", []string{"feat: rework events\n\nBREAKING CHANGE: ref layout changed"}, BumpMajor},
		{"breaking footer hyphen variant", []string{"fix: x\n\nBREAKING-CHANGE: y"}, BumpMajor},
		{"docs only is none", []string{"docs: tidy README"}, BumpNone},
		{"refactor only is none", []string{"refactor: extract adapter"}, BumpNone},
		{"chore/ci/test are none", []string{"chore: deps", "ci: cache go", "test: add table"}, BumpNone},
		{"non-conventional is none", []string{"add per-ISO-week dividers"}, BumpNone},
		{"highest wins: feat over fix", []string{"fix: a", "feat: b"}, BumpMinor},
		{"highest wins: breaking over feat", []string{"feat: a", "fix!: b"}, BumpMajor},
		{"BREAKING CHANGE only in body, not subject substring", []string{"fix: mention BREAKING CHANGE in passing on one line"}, BumpPatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := BumpFor(c.messages); got != c.want {
				t.Errorf("BumpFor(%q) = %v, want %v", c.messages, got, c.want)
			}
		})
	}
}

// TestNext computes the next version tag from the latest tag and a bump.
func TestNext(t *testing.T) {
	cases := []struct {
		latest string
		bump   Bump
		want   string
	}{
		{"v1.2.3", BumpPatch, "v1.2.4"},
		{"v1.2.3", BumpMinor, "v1.3.0"},
		{"v1.2.3", BumpMajor, "v2.0.0"},
		{"v1.2.3", BumpNone, ""},
		{"v0.1.2", BumpPatch, "v0.1.3"},
		// no prior tag → base at v0.0.0
		{"", BumpPatch, "v0.0.1"},
		{"", BumpMinor, "v0.1.0"},
		{"", BumpMajor, "v1.0.0"},
		{"", BumpNone, ""},
	}
	for _, c := range cases {
		got, err := Next(c.latest, c.bump)
		if err != nil {
			t.Fatalf("Next(%q, %v) errored: %v", c.latest, c.bump, err)
		}
		if got != c.want {
			t.Errorf("Next(%q, %v) = %q, want %q", c.latest, c.bump, got, c.want)
		}
	}
}

// TestNext_RejectsMalformed ensures a non-semver latest tag is an error rather
// than a silently-wrong version.
func TestNext_RejectsMalformed(t *testing.T) {
	for _, latest := range []string{"1.2.3", "v1.2", "vx.y.z", "v1.2.3.4"} {
		if _, err := Next(latest, BumpPatch); err == nil {
			t.Errorf("Next(%q, BumpPatch) should error", latest)
		}
	}
}
