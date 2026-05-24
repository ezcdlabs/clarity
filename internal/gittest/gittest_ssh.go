//go:build ssh

package gittest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewSSHRemote spins up an alpine+openssh+git container hosting a bare repo
// at /home/git/repo.git (pre-seeded with an initial commit on main), configures
// the test process to use a fixed private key for authentication, and returns
// a Remote whose URL points at the container's mapped SSH port.
//
// Cleanup (container teardown, env-var restore) is registered with t.Cleanup
// so the caller doesn't have to manage it.
//
// The returned Remote behaves like NewRemote's local-file equivalent for the
// methods that don't need on-disk repo access (URL(), NewClone()). Methods
// that DO touch the bare repo directly (LogBranch, ReadFileAtRef, ListRefs,
// DeleteRef) are local-only and would need a separate "exec inside the
// container" path to work over SSH — out of scope until a caller actually
// needs them.
//
// Build tag: this function is only compiled under `-tags ssh`. The lack of
// it under default builds keeps the test binary fast and means `go test
// ./...` doesn't need Docker.
func NewSSHRemote(t *testing.T) *Remote {
	t.Helper()
	ctx := context.Background()

	// Locate this file's directory so the Dockerfile build context resolves
	// regardless of where the test was invoked from. The keys/ subdirectory
	// next to gittest_ssh.go contains the private key we'll authenticate
	// with and the matching public key the Dockerfile bakes into the
	// server's authorized_keys.
	_, thisFile, _, _ := runtime.Caller(0)
	contextDir := filepath.Join(filepath.Dir(thisFile), "sshserver")
	privateKeyPath := filepath.Join(contextDir, "keys", "test_client_key")

	// SSH refuses to use a key file with overly-permissive permissions, but
	// git checkouts often clobber the original 0600 mode. Make a temp copy
	// with the correct permissions for this test run.
	keyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		t.Fatalf("read test client key: %v", err)
	}
	tmpKey := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(tmpKey, keyBytes, 0o600); err != nil {
		t.Fatalf("write tmp key: %v", err)
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    contextDir,
			Dockerfile: "Dockerfile",
			KeepImage:  true, // rebuild only when Dockerfile changes — big speedup on repeat runs
		},
		ExposedPorts: []string{"22/tcp"},
		WaitingFor:   wait.ForListeningPort("22/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start ssh container: %v", err)
	}
	t.Cleanup(func() {
		// 30s grace because some Docker setups are sluggish to terminate.
		_ = container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "22/tcp")
	if err != nil {
		t.Fatalf("container ssh port: %v", err)
	}

	// Build the ssh command git will use:
	//   - explicit -i to our test key (don't fall through to ~/.ssh/)
	//   - skip host-key checking entirely (each container has a fresh host
	//     key; persisting known_hosts would just warn on every run)
	//   - point at the mapped port the host OS sees
	sshCmd := fmt.Sprintf(
		"ssh -i %q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -p %d",
		tmpKey, port.Num(),
	)
	t.Setenv("GIT_SSH_COMMAND", sshCmd)

	url := fmt.Sprintf("ssh://git@%s:%d/home/git/repo.git", host, port.Num())
	return &Remote{url: url}
}
