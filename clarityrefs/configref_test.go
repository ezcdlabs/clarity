package clarityrefs_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestReadConfigFile_AbsentRef: a repo nobody has set up has no config ref,
// and that is its ordinary state — ok=false, no error, no bytes.
func TestReadConfigFile_AbsentRef(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	data, ok, err := clarityrefs.ReadConfigFile(clone.Path)
	if err != nil {
		t.Fatalf("ReadConfigFile on a repo with no config ref: %v", err)
	}
	if ok {
		t.Fatalf("ok=true for an absent ref; data=%q", data)
	}
}

// TestWriteConfigFile_RoundTripsThroughRemote: what one clone writes, a
// second clone reads back after a fetch — the whole point of keeping the
// config in a ref is that it travels like the events do.
func TestWriteConfigFile_RoundTripsThroughRemote(t *testing.T) {
	remote := gittest.NewRemote(t)
	writer := remote.NewClone(t)
	reader := remote.NewClone(t)

	content := []byte("[clarity]\n\tsource = events-ref\n")
	if err := clarityrefs.WriteConfigFile(writer.Path, "origin", content, "clarity: set up"); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	if err := clarityrefs.FetchConfigRef(reader.Path, "origin"); err != nil {
		t.Fatalf("FetchConfigRef: %v", err)
	}
	data, ok, err := clarityrefs.ReadConfigFile(reader.Path)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if !ok {
		t.Fatal("ok=false after a write and a fetch")
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("read back %q, want %q", data, content)
	}
}

// TestWriteConfigFile_SameContentIsIdempotent: re-running setup must be
// free — identical bytes produce no new commit and leave the remote ref
// where it was.
func TestWriteConfigFile_SameContentIsIdempotent(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	content := []byte("[clarity]\n\tsource = events-ref\n")
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", content, "clarity: set up"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	before := remoteRefSHA(t, remote.Path)

	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", content, "clarity: set up"); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if after := remoteRefSHA(t, remote.Path); after != before {
		t.Fatalf("identical content moved the ref: %s -> %s", before, after)
	}
}

// TestWriteConfigFile_ChangedContentChainsHistory: a config change commits
// on top of the previous config, so the ref carries an audit trail — who
// changed what, when — in plain git history.
func TestWriteConfigFile_ChangedContentChainsHistory(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", []byte("[clarity]\n\tsource = events-ref\n"), "clarity: set up"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := clarityrefs.WriteConfigFile(clone.Path, "origin", []byte("[clarity]\n\tsource = events-ref\n\tleadTime = reported\n"), "clarity: config change"); err != nil {
		t.Fatalf("second write: %v", err)
	}

	cmd := exec.Command("git", "rev-list", "--count", clarityrefs.ConfigRef)
	cmd.Dir = remote.Path
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "2" {
		t.Fatalf("expected 2 commits on the config ref, got %s", got)
	}
}

// TestFetchConfigRef_NoRemoteRefIsBenign: fetching the config ref from a
// remote that has none is every not-yet-set-up repo, not a failure.
func TestFetchConfigRef_NoRemoteRefIsBenign(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	if err := clarityrefs.FetchConfigRef(clone.Path, "origin"); err != nil {
		t.Fatalf("FetchConfigRef with no remote ref: %v", err)
	}
}

func remoteRefSHA(t *testing.T, remotePath string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", clarityrefs.ConfigRef)
	cmd.Dir = remotePath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse %s in remote: %v", clarityrefs.ConfigRef, err)
	}
	return strings.TrimSpace(string(out))
}
