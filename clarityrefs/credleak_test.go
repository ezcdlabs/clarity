package clarityrefs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// A credential embedded in the remote reaches a build log through git's own
// output, not only through clarity's message — git redacts the password from
// its diagnostics but prints the username in the clear, and
// `https://<token>@host` carries the token there.
//
// gitenv.Redact is unit-tested thoroughly, but a redactor that exists and is
// not called is exactly the shape of the bug that shipped once already. These
// tests pin the wiring: they drive the two paths that actually run in CI and
// assert the token is absent from what a user would see.
//
// Each one first proves git *would* leak it, so a change to git's wording
// turns the test into a skip rather than a silent pass.

const leakToken = "ghs_SUPERSECRETTOKEN"

// unauthorizedRemote serves a Basic-auth challenge to every request, which is
// what makes git emit "could not read Password for '<url>'" — the message
// that carries the username, and the one a misconfigured runner produces
// because clarity runs git with GIT_TERMINAL_PROMPT=0.
func unauthorizedRemote(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	return "http://" + leakToken + "@" + host + "/o/r.git"
}

// repoWithRemote creates a repository with an events ref to push and the
// given remote configured.
func repoWithRemote(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "credential.helper", ""},
		{"commit", "--allow-empty", "-m", "seed"},
		{"update-ref", EventsRef, "HEAD"},
		{"remote", "add", "origin", remote},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// assertGitWouldLeak runs the raw git command and requires its output to
// contain the token, so the redaction assertion below is testing something.
func assertGitWouldLeak(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), leakToken) {
		t.Skipf("this git does not put the credential in its output, so there is nothing to redact here:\n%s", out)
	}
}

func TestFetchEventsRef_DoesNotLeakCredentials(t *testing.T) {
	remote := unauthorizedRemote(t)
	dir := repoWithRemote(t, remote)
	assertGitWouldLeak(t, dir, "fetch", "origin", "+"+EventsRef+":"+EventsRef)

	err := fetchEventsRef(dir, "origin")
	if err == nil {
		t.Fatal("fetching from a remote that refuses auth should fail")
	}
	if strings.Contains(err.Error(), leakToken) {
		t.Errorf("the fetch error leaked the credential:\n%s", err)
	}
}

func TestPushEventsRef_DoesNotLeakCredentials(t *testing.T) {
	remote := unauthorizedRemote(t)
	dir := repoWithRemote(t, remote)
	assertGitWouldLeak(t, dir, "push", "origin", EventsRef+":"+EventsRef)

	err := pushEventsRef(dir, "origin")
	if err == nil {
		t.Fatal("pushing to a remote that refuses auth should fail")
	}
	if strings.Contains(err.Error(), leakToken) {
		t.Errorf("the push error leaked the credential:\n%s", err)
	}
}

// TestRemoteURL_DoesNotLeakCredentials covers the other half of the message:
// the URL clarity names itself, which comes from `git remote get-url` and is
// returned verbatim by git.
func TestRemoteURL_DoesNotLeakCredentials(t *testing.T) {
	remote := unauthorizedRemote(t)
	dir := repoWithRemote(t, remote)

	if got := remoteURL(dir, "origin"); strings.Contains(got, leakToken) {
		t.Errorf("remoteURL leaked the credential: %s", got)
	}
	if got := remoteURL(dir, "origin"); !strings.Contains(got, "127.0.0.1") {
		t.Errorf("remoteURL should still name the host, got: %s", got)
	}
}
