// Package trust records the user's approval of an exec source command.
//
// The model is direnv's `allow` / mise's `trust`: the configuration that
// names the command is shared (`.clarity` on the trunk), so it must never
// run without this clone's human having seen and approved the exact argv.
// The approval is a hash of that argv stored in the repository's LOCAL git
// config — `.git/config` travels with no push and no fetch, so a fresh
// clone always asks its own human again, and any change to the configured
// command invalidates the approval by construction.
package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ezcdlabs/clarity/internal/gitenv"
)

// trustedKey holds the fingerprint of the approved argv.
const trustedKey = "clarity.source.trusted"

// commandKey holds a display copy of what was approved, so a changed
// command can be shown next to what it changed from. Display only: the
// fingerprint is what is verified, never this string.
const commandKey = "clarity.source.trustedcommand"

// Fingerprint names an argv unambiguously: sha256 over the arguments
// joined with NUL, prefixed with the scheme. NUL cannot appear inside an
// argument, so argument boundaries are part of the identity — ["ab","c"]
// and ["a","bc"] are different commands and hash differently. A leading
// count folds in the empty-argv/one-empty-arg distinction too.
func Fingerprint(argv []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d\x00", len(argv))
	for _, a := range argv {
		h.Write([]byte(a))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// IsTrusted reports whether this exact argv is the one this clone's human
// approved. No recorded approval, or an approval of anything else, is
// false — the mechanism has no notion of "close enough".
func IsTrusted(repoPath string, argv []string) (bool, error) {
	recorded, ok, err := localConfig(repoPath, trustedKey)
	if err != nil || !ok {
		return false, err
	}
	return recorded == Fingerprint(argv), nil
}

// Record stores the approval of this exact argv, replacing any previous
// one. Callers show the argv and collect the typed confirmation first;
// this is only the write.
func Record(repoPath string, argv []string) error {
	if err := setLocalConfig(repoPath, trustedKey, Fingerprint(argv)); err != nil {
		return err
	}
	return setLocalConfig(repoPath, commandKey, Display(argv))
}

// RecordedCommand returns the display form of the approved command, for
// showing what a changed configuration changed from. ok=false means no
// approval has ever been recorded here.
func RecordedCommand(repoPath string) (string, bool) {
	cmd, ok, err := localConfig(repoPath, commandKey)
	if err != nil || !ok {
		return "", false
	}
	return cmd, true
}

// Display renders an argv for human eyes, one unambiguous line. Arguments
// carrying spaces or quotes are quoted Go-style so what is shown cannot be
// misread as different argument boundaries than what runs.
func Display(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			parts = append(parts, fmt.Sprintf("%q", a))
			continue
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// localConfig reads one key from the repository's local git config.
// ok=false means the key is unset, which is every repo before its first
// approval.
func localConfig(repoPath, key string) (string, bool, error) {
	cmd := exec.Command("git", "config", "--local", "--get", key)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", false, nil // unset
		}
		return "", false, fmt.Errorf("read %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), true, nil
}

func setLocalConfig(repoPath, key, value string) error {
	cmd := exec.Command("git", "config", "--local", key, value)
	cmd.Dir = repoPath
	cmd.Env = gitenv.Clean()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("write %s: %w\n%s", key, err, gitenv.Redact(string(out)))
	}
	return nil
}
