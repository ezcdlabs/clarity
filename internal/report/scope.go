package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
)

// ScopeOptions configures one candidacy report.
type ScopeOptions struct {
	RepoPath string
	Remote   string    // defaults to "origin"
	Target   string    // required
	Affected bool      // whether this commit contains change that target ships
	Time     time.Time // defaults to time.Now()
	SHA      string    // defaults to env (GITHUB_SHA / CI_COMMIT_SHA), then HEAD
}

// validateScope checks a candidacy target. The format rules are the deploy
// target's, because the records share a ref and a permanence — but here the
// target is required rather than optional: "affected" with nothing to be
// affected of is not a statement about anything.
func validateScope(target string) error {
	if target == "" {
		return fmt.Errorf("target is required: git clarity report affected|unaffected <target>")
	}
	return validateTargetFormat(target)
}

// ValidateScope is validateScope for callers outside the package, so the CLI
// can refuse an invocation before echoing or writing anything.
func ValidateScope(opts ScopeOptions) error { return validateScope(opts.Target) }

// ScopeCommandLine renders the fully-explicit form of a candidacy report, for
// the same reason stage reporting echoes one: a dropped record leaves a flow's
// lead time wrong, and re-running a bare command later would record today's
// fact at a later timestamp.
func ScopeCommandLine(opts ScopeOptions) string {
	return fmt.Sprintf("git clarity report --sha %s --at %s %s %s",
		opts.SHA,
		opts.Time.UTC().Truncate(time.Second).Format(time.RFC3339),
		scopeVerb(opts.Affected),
		opts.Target,
	)
}

func scopeVerb(affected bool) string {
	if affected {
		return "affected"
	}
	return "unaffected"
}

// ResolveScope fills in the SHA and timestamp the same way Resolve does for
// events, so both grammars infer them identically.
func ResolveScope(opts ScopeOptions) (ScopeOptions, error) {
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if opts.Time.IsZero() {
		opts.Time = time.Now()
	}
	if opts.SHA == "" {
		sha, err := resolveSHA(opts.RepoPath)
		if err != nil {
			return ScopeOptions{}, err
		}
		opts.SHA = sha
	}
	return opts, nil
}

// RunScope records one commit's candidacy for one target.
func RunScope(opts ScopeOptions) (string, error) {
	if err := validateScope(opts.Target); err != nil {
		return "", err
	}
	opts, err := ResolveScope(opts)
	if err != nil {
		return "", err
	}
	scope := clarityrefs.Scope{Target: opts.Target, Affected: opts.Affected, Time: opts.Time}
	if err := clarityrefs.WriteScope(opts.RepoPath, opts.Remote, opts.SHA, scope); err != nil {
		return "", err
	}
	return opts.SHA, nil
}

// ScopeFailureError wraps a failed candidacy write with the command that
// reproduces it, for the same reason stage reporting does: the pre-write echo
// is gone the moment a step is killed or times out, and a red log is where
// someone is already looking.
//
// A dropped candidacy record fails quietly rather than loudly — nothing shows
// as in-flight, the flow just keeps counting a commit it doesn't ship, and its
// lead time stays wrong with no sign that anything was lost.
func ScopeFailureError(opts ScopeOptions, err error) error {
	cause := strings.TrimSpace(err.Error())
	return &scopeFailureError{
		cause: err,
		msg: fmt.Sprintf(
			"failed to report %s %s: %s\n\n"+
				"Candidacy was not recorded for %s. Nothing will look broken — the\n"+
				"%s flow will simply keep counting this commit towards its lead time.\n"+
				"Re-run this once the problem is fixed:\n\n"+
				"  %s\n",
			scopeVerb(opts.Affected), opts.Target, cause,
			shortSHA(opts.SHA), opts.Target,
			ScopeCommandLine(opts),
		),
	}
}

type scopeFailureError struct {
	cause error
	msg   string
}

func (e *scopeFailureError) Error() string { return e.msg }
func (e *scopeFailureError) Unwrap() error { return e.cause }
