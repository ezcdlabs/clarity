// Package core is the FFI surface gomobile binds for the mobile apps.
//
// Everything exported here fits gomobile's supported subset: strings, bytes,
// ints, bools, errors, and pointers to structs. Nothing richer crosses —
// structured data goes over as encoded protobuf, which sidesteps the type
// system entirely and keeps one schema generating all three sides rather than
// a hand-written wrapper per platform that can drift.
//
// There is no logic here. Every method is a translation of an FFI-friendly
// shape into a call on mobile/internal/*, which is where the behaviour lives
// and where it can be tested without a device.
package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/mobile/internal/gitsource"
	"github.com/ezcdlabs/clarity/mobile/internal/hostkeys"
	"github.com/ezcdlabs/clarity/mobile/internal/keys"
	"github.com/ezcdlabs/clarity/mobile/internal/present"
	"github.com/ezcdlabs/clarity/mobile/internal/registry"
	"github.com/ezcdlabs/clarity/mobile/internal/remote"
	"github.com/ezcdlabs/clarity/mobile/internal/watch"
	v1 "github.com/ezcdlabs/clarity/proto/gen/go/clarityv1"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"google.golang.org/protobuf/proto"
)

// Metrics returns an encoded clarity.v1.Metrics for a repository: the weekly
// aggregates view, read from what the last Sync fetched. Never touches the
// network.
//
// Separate from View, and read over a far larger commit window, because it
// answers a different question. View answers "is main green right now?" and is
// re-read every few seconds; this answers "are we getting better?", which is
// about history — so it is read when the screen opens and not again.
//
// weeks is the window, in whole weeks rather than in commits, so how far back a
// reader can see does not depend on how busy the repository was: a commit limit
// gives a quiet repo a year and a busy one four days, which is useless for
// comparing trend. commitLimit bounds the read itself; 0 means unlimited, as it
// does on every other entry point.
func (c *Client) Metrics(repoID string, commitLimit, weeks int) ([]byte, error) {
	if weeks < 1 {
		return nil, fmt.Errorf("weeks must be at least 1, got %d", weeks)
	}
	repo, _, err := c.open(repoID)
	if err != nil {
		return nil, err
	}
	snap, err := repo.Snapshot(commitLimit)
	if err != nil {
		return nil, err
	}
	cfg, err := repo.Config()
	if err != nil {
		return nil, err
	}
	// The same derivation and the same trimming the CLI runs, so a week reads
	// the same on a phone as it does in a terminal — including recomputing the
	// axis after narrowing, without which the scale claims to be hiding data
	// from weeks nobody can see.
	view := core.TrimToWholeWeeks(core.DeriveView(snap, cfg.LeadTimeMode(), cfg.Deploys()), weeks)
	return proto.Marshal(present.Metrics(view, time.Now()))
}

// Elapsed formats a number of seconds the way every clarity UI formats a
// duration.
//
// Exposed because a running timer has to be recomputed on the device every
// second, and asking Go for the whole view that often would mean walking the
// commit graph that often. The alternative was reimplementing the rule in
// Kotlin and in Swift — two more places for "3m 29s" to drift into "3:29".
//
// Negative input reads as zero. A phone's clock and a CI host's clock
// disagree, and a timer counting backwards from a deploy that has not happened
// yet reads as a bug in clarity rather than a skew.
func Elapsed(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return core.FormatElapsed(time.Duration(seconds) * time.Second)
}

// Client is the whole surface an app talks to. One per process; the app
// supplies the directory its platform gives it for private data.
type Client struct {
	dir      string
	identity *keys.Identity
	repos    *registry.Registry
	hosts    *hostkeys.Store
}

// New prepares a client over a data directory. Nothing is read or generated
// until it is needed, so this is safe to call during app startup.
func New(dataDir string) (*Client, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("a data directory is required")
	}
	return &Client{
		dir:      dataDir,
		identity: keys.Open(filepath.Join(dataDir, "identity")),
		repos:    registry.Open(dataDir),
		hosts:    hostkeys.Open(filepath.Join(dataDir, "known_hosts")),
	}, nil
}

// PublicKey returns the authorized_keys line for this device, generating the
// keypair on first call. The user pastes it into whichever host they use —
// there is no provider to connect to and no OAuth app to register.
func (c *Client) PublicKey(comment string) (string, error) {
	return c.identity.PublicKey(comment)
}

// HasKey reports whether an identity exists yet, without creating one.
func (c *Client) HasKey() bool { return c.identity.Exists() }

// AddRepo tracks a repository. The URL is the one you would clone.
// Returns its id.
func (c *Client) AddRepo(url, branch string) (string, error) {
	e, err := c.repos.Add(url, branch)
	if err != nil {
		return "", err
	}
	return e.ID, nil
}

// RemoveRepo forgets a repository. Its object store is left on disk for the
// caller to delete, because deleting user data is not something a bridge
// method should do as a side effect.
func (c *Client) RemoveRepo(repoID string) error { return c.repos.Remove(repoID) }

// StorePath is where a repository's objects live, so an app that wants to
// reclaim the space after RemoveRepo knows what to delete.
func (c *Client) StorePath(repoID string) string { return c.repos.StorePath(repoID) }

// ListRepos returns an encoded clarity.v1.RepoList.
func (c *Client) ListRepos() ([]byte, error) {
	entries, err := c.repos.List()
	if err != nil {
		return nil, err
	}
	out := &v1.RepoList{GeneratedUnixSeconds: time.Now().Unix()}
	for _, e := range entries {
		out.Repos = append(out.Repos, summary(e))
	}
	return proto.Marshal(out)
}

// Rename gives a repository a different title on this device. A blank name
// clears the rename and the URL's own name comes back.
func (c *Client) Rename(repoID, name string) error {
	return c.repos.SetAlias(repoID, name)
}

// SetBranch changes which branch a repository watches.
func (c *Client) SetBranch(repoID, branch string) error {
	return c.repos.SetBranch(repoID, branch)
}

// summary labels a tracked repository for a list.
//
// The name, namespace and host are derived here rather than stored, so a
// change to the rule applies to every repository already added instead of only
// to the next one. The alias and the cached verdicts are facts and come from
// the registry.
func summary(e registry.Entry) *v1.RepoSummary {
	ref := remote.Parse(e.URL)
	out := &v1.RepoSummary{
		Id:        e.ID,
		Name:      ref.Name,
		Namespace: ref.Namespace,
		Host:      ref.Host,
		Alias:     e.Alias,
		Url:       e.URL,
		Branch:    e.Branch,
	}
	if e.Status != nil {
		out.Ci = present.Status(e.Status.CI)
		out.Deploy = present.Status(e.Status.Deploy)
		for _, f := range e.Status.Flows {
			out.Flows = append(out.Flows, &v1.FlowSummary{
				Name: f.Name, Deploy: present.Status(f.Deploy),
			})
		}
	}
	return out
}

// Sync fetches a repository, returning an encoded clarity.v1.SyncResult.
//
// Outcomes rather than an exception, because a fetch has failures a UI must
// tell apart and a thrown message cannot: a host nobody has agreed to yet is a
// question with its own dialog, a rejected key is a fixable setup step with
// its own screen, and everything else is a message. The alternative is each
// client matching on the text of an error, which is a parser for prose that
// breaks when git rewords something.
//
// Blocking: the caller runs it off the UI thread, which every mobile platform
// requires of network work anyway.
//
// timeoutSeconds bounds the fetch rather than the session. Zero means a
// default; the view that follows holds a snapshot and has nothing left to
// time out.
func (c *Client) Sync(repoID string, depth, timeoutSeconds int) ([]byte, error) {
	repo, entry, err := c.open(repoID)
	if err != nil {
		return nil, err
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	auth, err := c.auth(entry.URL)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(outcome(repo.Sync(ctx, gitsource.SyncOptions{Depth: depth, Auth: auth})))
}

// Check fetches every tracked repository and reports the pipelines that have
// crossed between green and red since the last check.
//
// This is what a background worker calls. It returns transitions rather than
// state on purpose: a check runs on a timer whether or not anything happened,
// and a notification keyed on state fires for as long as a build stays broken.
//
// One repository failing does not fail the check. A phone is offline half the
// time and a host can be down; the repositories that were reached still have
// news worth hearing, and the count of those that were not is carried so a
// caller can tell "nothing is wrong" from "nothing could be read".
func (c *Client) Check(timeoutSeconds int) ([]byte, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	entries, err := c.repos.List()
	if err != nil {
		return nil, err
	}

	out := &v1.Changes{GeneratedUnixSeconds: time.Now().Unix()}
	for _, entry := range entries {
		changes, err := c.checkOne(entry, timeoutSeconds)
		if err != nil {
			out.Unreachable++
			continue
		}
		out.Checked++
		for _, ch := range changes {
			out.Changes = append(out.Changes, &v1.Change{
				RepoId:   entry.ID,
				RepoName: label(entry),
				Stage:    ch.Stage,
				Broke:    ch.Broke,
				From:     present.Status(ch.From),
				To:       present.Status(ch.To),
			})
		}
	}
	return proto.Marshal(out)
}

// checkOne fetches a repository and compares what it now says to what was
// recorded last time, then records the new verdict.
//
// The order matters: the comparison has to read the stored verdict before the
// fresh one overwrites it, which is why this does not simply call View.
func (c *Client) checkOne(entry registry.Entry, timeoutSeconds int) ([]watch.Change, error) {
	repo, _, err := c.open(entry.ID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	auth, err := c.auth(entry.URL)
	if err != nil {
		return nil, err
	}
	if err := repo.Sync(ctx, gitsource.SyncOptions{Auth: auth}); err != nil {
		return nil, err
	}

	snap, err := repo.Snapshot(checkCommitWindow)
	if err != nil {
		return nil, err
	}
	cfg, err := repo.Config()
	if err != nil {
		return nil, err
	}
	view := core.DeriveView(snap, cfg.LeadTimeMode(), cfg.Deploys())

	changes := watch.Compare(entry.Status, verdict(view))
	c.remember(entry.ID, view)
	return changes, nil
}

// checkCommitWindow is how much history a background check reads. Small: it
// only needs the head of each pipeline, and this runs on a timer.
const checkCommitWindow = 50

// label is what a notification calls a repository: its rename if it has one,
// otherwise the name derived from the URL — the same label the switcher shows.
func label(e registry.Entry) string {
	if e.Alias != "" {
		return e.Alias
	}
	ref := remote.Parse(e.URL)
	if ref.Namespace == "" {
		return ref.Name
	}
	return ref.Namespace + "/" + ref.Name
}

// TrustHost records a host key the user agreed to in the prompt, so the fetch
// that was refused can be retried.
func (c *Client) TrustHost(host, fingerprint string) error {
	return c.hosts.Trust(host, fingerprint)
}

// outcome sorts what a fetch returned into the cases a UI draws differently.
func outcome(err error) *v1.SyncResult {
	if err == nil {
		return &v1.SyncResult{Outcome: v1.Outcome_OUTCOME_OK}
	}

	var unknown *hostkeys.UnknownHostError
	if errors.As(err, &unknown) {
		return &v1.SyncResult{
			Outcome: v1.Outcome_OUTCOME_HOST_KEY_UNKNOWN,
			Message: unknown.Error(),
			HostKey: &v1.HostKey{
				Host: unknown.Host, Type: unknown.Type, Fingerprint: unknown.Fingerprint,
			},
		}
	}
	var changed *hostkeys.ChangedHostError
	if errors.As(err, &changed) {
		return &v1.SyncResult{
			Outcome: v1.Outcome_OUTCOME_HOST_KEY_CHANGED,
			Message: changed.Error(),
			HostKey: &v1.HostKey{
				Host: changed.Host, Type: changed.Type, Fingerprint: changed.Fingerprint,
			},
		}
	}
	if isAuthDenied(err) {
		return &v1.SyncResult{
			Outcome: v1.Outcome_OUTCOME_AUTH_DENIED,
			Message: "the host did not accept this device's key",
			// The raw thing git said, for the disclosure. Shown rather than
			// summarised: whoever is debugging a key wants the actual words.
			GitOutput: err.Error(),
		}
	}
	return &v1.SyncResult{Outcome: v1.Outcome_OUTCOME_FAILED, Message: err.Error()}
}

// isAuthDenied recognises the host refusing this device's key.
//
// By substring, which is not something to be proud of — but ssh's failure
// arrives as a formatted string through two libraries, and there is no typed
// error to match on. The strings are the stable parts of the message: the list
// of attempted methods varies, the phrase does not.
func isAuthDenied(err error) bool {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "unable to authenticate"):
		return true
	case strings.Contains(s, "permission denied"):
		return true
	case strings.Contains(s, "handshake failed") && strings.Contains(s, "publickey"):
		return true
	default:
		return false
	}
}

// View returns an encoded clarity.v1.View// View returns an encoded clarity.v1.View for a repository, read from what
// the last Sync fetched. Never touches the network.
func (c *Client) View(repoID string, limit int) ([]byte, error) {
	repo, _, err := c.open(repoID)
	if err != nil {
		return nil, err
	}
	snap, err := repo.Snapshot(limit)
	if err != nil {
		return nil, err
	}
	cfg, err := repo.Config()
	if err != nil {
		return nil, err
	}
	// The same derivation the CLI runs, from the same config — so a repo
	// reads the same on a phone as it does in a terminal.
	view := core.DeriveView(snap, cfg.LeadTimeMode(), cfg.Deploys())
	c.remember(repoID, view)
	return proto.Marshal(present.View(view, time.Now()))
}

// remember writes down what a view said, so a switcher can label every
// repository without opening each one.
//
// Best effort on purpose: a label in a list is not worth failing the view the
// user actually asked for.
func (c *Client) remember(repoID string, view core.View) {
	_ = c.repos.SetStatus(repoID, verdict(view))
}

// verdict is what a view says, in the shape the registry stores and the
// background check compares. One function, because a verdict recorded in one
// shape and compared in another would report a change every time the app and
// the worker took turns.
func verdict(view core.View) registry.Status {
	st := registry.Status{CI: view.Header.CI, Deploy: view.Header.Deploy}
	for _, f := range view.Flows {
		st.Flows = append(st.Flows, registry.FlowStatus{Name: f.Name, Deploy: f.Deploy})
	}
	return st
}

// open prepares the git store for a tracked repository.
func (c *Client) open(repoID string) (*gitsource.Repo, registry.Entry, error) {
	entry, err := c.repos.Get(repoID)
	if err != nil {
		return nil, registry.Entry{}, err
	}
	fs := osfs.New(c.repos.StorePath(entry.ID))
	st := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	repo, err := gitsource.Open(st, entry.URL, entry.Branch)
	if err != nil {
		return nil, registry.Entry{}, err
	}
	return repo, entry, nil
}

// auth supplies the device key for ssh remotes. An https remote gets none:
// a public repository needs no credential, and clarity has nowhere to put a
// password it has not been given.
func (c *Client) auth(url string) (*gogitssh.PublicKeys, error) {
	if !isSSH(url) {
		return nil, nil
	}
	signer, err := c.identity.Ensure()
	if err != nil {
		return nil, err
	}
	auth := &gogitssh.PublicKeys{User: sshUser(url), Signer: signer}
	// Without this, go-git looks for ~/.ssh/known_hosts and fails before it
	// reaches the network: a phone has no ssh client and no such file. The app
	// keeps its own, and hostkeys explains what it does with it.
	auth.HostKeyCallback = c.hosts.Callback()
	return auth, nil
}

func isSSH(url string) bool {
	return hasPrefix(url, "ssh://") || (containsAt(url) && !hasPrefix(url, "http://") && !hasPrefix(url, "https://"))
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func containsAt(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			return true
		}
	}
	return false
}

// sshUser is the user an ssh remote names, defaulting to git — which is what
// every host clarity targets uses.
func sshUser(url string) string {
	s := url
	if hasPrefix(s, "ssh://") {
		s = s[len("ssh://"):]
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			if i > 0 {
				return s[:i]
			}
			break
		}
	}
	return "git"
}
