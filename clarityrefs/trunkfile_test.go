package clarityrefs_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/gittest"
)

// TestReadTrunkFile_ReadsTheCommittedFile: a file committed and pushed to
// the trunk is read back out of the remote-tracking tip, with no checkout
// involved.
func TestReadTrunkFile_ReadsTheCommittedFile(t *testing.T) {
	remote := gittest.NewRemote(t)
	writer := remote.NewClone(t)
	writer.WriteFile(".clarity", "[clarity]\n\tleadTime = reported\n")
	writer.CommitAll("add .clarity")
	writer.Push("main")

	reader := remote.NewClone(t)
	data, ok, err := clarityrefs.ReadTrunkFile(reader.Path, "main", ".clarity")
	if err != nil {
		t.Fatalf("ReadTrunkFile: %v", err)
	}
	if !ok {
		t.Fatal("ok = false for a file that is on the trunk")
	}
	if string(data) != "[clarity]\n\tleadTime = reported\n" {
		t.Errorf("data = %q, want the committed bytes", data)
	}
}

// TestReadTrunkFile_WorkingTreeHasNoEffect: a file that exists only in the
// working tree is not on the trunk. Checking out a colleague's branch, or
// editing the file locally, must not change what clarity reads.
func TestReadTrunkFile_WorkingTreeHasNoEffect(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile(".clarity", "[clarity]\n\tleadTime = all\n")

	_, ok, err := clarityrefs.ReadTrunkFile(clone.Path, "main", ".clarity")
	if err != nil {
		t.Fatalf("ReadTrunkFile: %v", err)
	}
	if ok {
		t.Error("ok = true for a file that exists only in the working tree")
	}
}

// TestReadTrunkFile_RemoteTrackingOutranksLocal: a commit on the local
// trunk branch that has not been pushed is not shared truth yet. The
// remote-tracking ref is authoritative whenever it exists.
func TestReadTrunkFile_RemoteTrackingOutranksLocal(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)
	clone.WriteFile(".clarity", "[clarity]\n\tleadTime = all\n")
	clone.CommitAll("add .clarity locally") // committed on local main, never pushed

	_, ok, err := clarityrefs.ReadTrunkFile(clone.Path, "main", ".clarity")
	if err != nil {
		t.Fatalf("ReadTrunkFile: %v", err)
	}
	if ok {
		t.Error("ok = true from an unpushed local commit while origin/main lacks the file")
	}
}

// TestReadTrunkFile_LocalBranchServesARepoWithNoRemote: a repository that
// has never been pushed anywhere still has a trunk; the local branch tip is
// all there is, so it answers.
func TestReadTrunkFile_LocalBranchServesARepoWithNoRemote(t *testing.T) {
	repo := gittest.NewLocalRepo(t, "main")
	repo.WriteFile(".clarity", "[clarity]\n\tleadTime = reported\n")
	repo.CommitAll("add .clarity")

	data, ok, err := clarityrefs.ReadTrunkFile(repo.Path, "main", ".clarity")
	if err != nil {
		t.Fatalf("ReadTrunkFile: %v", err)
	}
	if !ok {
		t.Fatal("ok = false with the file committed on the only trunk there is")
	}
	if string(data) != "[clarity]\n\tleadTime = reported\n" {
		t.Errorf("data = %q, want the committed bytes", data)
	}
}

// TestReadTrunkFile_MissingFileAndMissingBranch: both ordinary states, not
// errors — most repos have no .clarity, and a misdetected trunk resolves
// nowhere.
func TestReadTrunkFile_MissingFileAndMissingBranch(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	if _, ok, err := clarityrefs.ReadTrunkFile(clone.Path, "main", ".clarity"); err != nil || ok {
		t.Errorf("missing file: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if _, ok, err := clarityrefs.ReadTrunkFile(clone.Path, "no-such-branch", ".clarity"); err != nil || ok {
		t.Errorf("missing branch: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}
