package trust_test

import (
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/trust"
)

// TestFingerprint pins the one property the whole mechanism rests on: two
// argvs are the same command if and only if they fingerprint the same.
// Argument boundaries are part of identity — ["ab","c"] and ["a","bc"]
// run different things — which is why the hash is over a delimited join,
// not a concatenation.
func TestFingerprint(t *testing.T) {
	cases := map[string]struct {
		a, b []string
		same bool
	}{
		"identical argvs match":               {[]string{"./helper", "--x"}, []string{"./helper", "--x"}, true},
		"a changed argument differs":          {[]string{"./helper", "--x"}, []string{"./helper", "--y"}, false},
		"an added argument differs":           {[]string{"./helper"}, []string{"./helper", "--x"}, false},
		"argument boundaries are identity":    {[]string{"ab", "c"}, []string{"a", "bc"}, false},
		"order is identity":                   {[]string{"a", "b"}, []string{"b", "a"}, false},
		"empty argv and one empty arg differ": {[]string{}, []string{""}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fa, fb := trust.Fingerprint(tc.a), trust.Fingerprint(tc.b)
			if (fa == fb) != tc.same {
				t.Errorf("Fingerprint(%q) = %s, Fingerprint(%q) = %s — same=%v, want %v",
					tc.a, fa, tc.b, fb, fa == fb, tc.same)
			}
		})
	}
	if fp := trust.Fingerprint([]string{"./helper"}); !strings.HasPrefix(fp, "sha256:") {
		t.Errorf("Fingerprint = %q, want a sha256: prefix naming the scheme", fp)
	}
}

// TestRecordAndIsTrusted: an approval round-trips through the repository's
// LOCAL git config — and only the exact argv it was given is trusted.
func TestRecordAndIsTrusted(t *testing.T) {
	repo := gittest.NewLocalRepo(t, "main")
	argv := []string{"./tools/helper", "--store=ch"}

	if ok, err := trust.IsTrusted(repo.Path, argv); err != nil || ok {
		t.Fatalf("fresh repo: trusted=%v err=%v, want untrusted", ok, err)
	}
	if err := trust.Record(repo.Path, argv); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if ok, err := trust.IsTrusted(repo.Path, argv); err != nil || !ok {
		t.Fatalf("after Record: trusted=%v err=%v, want trusted", ok, err)
	}
	if ok, _ := trust.IsTrusted(repo.Path, []string{"./tools/helper", "--store=pg"}); ok {
		t.Fatal("a changed argument is still trusted — the approval must pin the exact argv")
	}
	if cmd, ok := trust.RecordedCommand(repo.Path); !ok || !strings.Contains(cmd, "--store=ch") {
		t.Errorf("RecordedCommand = %q, %v — want the display form of what was approved", cmd, ok)
	}
}

// TestRecordIsLocalOnly: the approval must live in .git/config, which no
// push carries — a fresh clone has to ask its own human again.
func TestRecordIsLocalOnly(t *testing.T) {
	remote := gittest.NewRemote(t)
	first := remote.NewClone(t)
	argv := []string{"./helper"}
	if err := trust.Record(first.Path, argv); err != nil {
		t.Fatalf("Record: %v", err)
	}
	first.WriteFile("f.txt", "x")
	first.CommitAll("work")
	first.Push("main")

	second := remote.NewClone(t)
	if ok, _ := trust.IsTrusted(second.Path, argv); ok {
		t.Fatal("a fresh clone inherited the approval — it must be local-only")
	}
}
