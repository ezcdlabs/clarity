package refs_test

import (
	"testing"

	"github.com/ezcdlabs/clarity/internal/gittest"
	"github.com/ezcdlabs/clarity/internal/refs"
)

func TestPushRef_CreatesRefOnRemote(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	clone.WriteFile("file.txt", "content")
	clone.CommitAll("some commit")

	if err := refs.PushRef(clone.Path, "origin", "HEAD", "refs/clarity/test-ref"); err != nil {
		t.Fatalf("PushRef failed: %v", err)
	}

	found := false
	for _, r := range remote.ListRefs() {
		if r == "refs/clarity/test-ref" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected refs/clarity/test-ref on remote after PushRef")
	}
}

func TestDeleteRef_RemovesRefFromRemote(t *testing.T) {
	remote := gittest.NewRemote(t)
	clone := remote.NewClone(t)

	clone.WriteFile("file.txt", "content")
	clone.CommitAll("some commit")
	clone.PushRef("HEAD", "refs/clarity/test-ref")

	if err := refs.DeleteRef(clone.Path, "origin", "refs/clarity/test-ref"); err != nil {
		t.Fatalf("DeleteRef failed: %v", err)
	}

	for _, r := range remote.ListRefs() {
		if r == "refs/clarity/test-ref" {
			t.Fatal("expected refs/clarity/test-ref to be deleted from remote")
		}
	}
}

func TestPushRef_FastForwardRejected_ReturnsError(t *testing.T) {
	remote := gittest.NewRemote(t)
	alice := remote.NewClone(t)
	bob := remote.NewClone(t)

	// Alice pushes a commit to a ref.
	alice.WriteFile("alice.txt", "alice")
	alice.CommitAll("alice's commit")
	alice.PushRef("HEAD", "refs/clarity/shared-ref")

	// Bob pushes a different commit to the same ref — not a fast-forward.
	bob.WriteFile("bob.txt", "bob")
	bob.CommitAll("bob's commit")

	err := refs.PushRef(bob.Path, "origin", "HEAD", "refs/clarity/shared-ref")
	if err == nil {
		t.Fatal("expected error on non-fast-forward push")
	}
}

func TestFetchRef_MakesRemoteRefAvailableLocally(t *testing.T) {
	remote := gittest.NewRemote(t)
	alice := remote.NewClone(t)
	bob := remote.NewClone(t)

	alice.WriteFile("alice.txt", "alice")
	alice.CommitAll("alice's commit")
	alice.PushRef("HEAD", "refs/clarity/alice-test")

	if err := refs.FetchRef(bob.Path, "origin", "refs/clarity/alice-test", "refs/clarity-fetched/alice-test"); err != nil {
		t.Fatalf("FetchRef failed: %v", err)
	}

	commits := bob.LogBranch("refs/clarity-fetched/alice-test")
	if len(commits) == 0 || commits[0].Message != "alice's commit" {
		t.Fatalf("expected alice's commit at fetched ref, got: %v", commits)
	}
}
