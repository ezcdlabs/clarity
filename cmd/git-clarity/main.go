package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezcdlabs/clarity/clarityrefs"
	"github.com/ezcdlabs/clarity/internal/adapters/plain"
	"github.com/ezcdlabs/clarity/internal/adapters/refsource"
	"github.com/ezcdlabs/clarity/internal/adapters/tui"
	"github.com/ezcdlabs/clarity/internal/cache"
	"github.com/ezcdlabs/clarity/internal/checkcmd"
	"github.com/ezcdlabs/clarity/internal/config"
	"github.com/ezcdlabs/clarity/internal/configcmd"
	"github.com/ezcdlabs/clarity/internal/core"
	"github.com/ezcdlabs/clarity/internal/gitenv"
	"github.com/ezcdlabs/clarity/internal/gitlog"
	"github.com/ezcdlabs/clarity/internal/initcmd"
	"github.com/ezcdlabs/clarity/internal/onboardcmd"
	"github.com/ezcdlabs/clarity/internal/report"
)

// version is set at build time via -ldflags "-X main.version=<value>".
var version = "dev"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	if err := dispatch(os.Args[1:]); err != nil {
		// A subcommand whose exit code is part of its interface reports it
		// this way, having already written its own output.
		var exit exitError
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// rootOptions are the flags accepted on the base `git clarity` invocation.
// Subcommands (currently just `report`) handle their own arg parsing and
// don't see these flags.
type rootOptions struct {
	plain    bool
	showSHAs bool
	limit    int    // 0 == unlimited (sentinel); default is 100
	cacheDir string // --cache-dir override; "" means env-or-default
	deploy   string // --deploy: open on / narrow to one deploy flow
}

func dispatch(args []string) error {
	// Subcommands consume their own args before any root-flag parsing.
	if len(args) > 0 && args[0] == "report" {
		return runReport(args[1:])
	}
	if len(args) > 0 && args[0] == "init" {
		return runInit(args[1:])
	}
	if len(args) > 0 && args[0] == "onboard" {
		return runOnboard(args[1:])
	}
	if len(args) > 0 && args[0] == "config" {
		repoPath, err := repoRoot()
		if err != nil {
			return fmt.Errorf("not a git repository: %w", err)
		}
		return configcmd.Run(configcmd.Options{RepoPath: repoPath, Out: os.Stdout, Args: args[1:]})
	}
	if len(args) > 0 && args[0] == "metrics" {
		return runMetrics(args[1:])
	}
	if len(args) > 0 && args[0] == "check" {
		return runCheck(args[1:])
	}
	opts, err := parseRootArgs(args)
	if err != nil {
		return err
	}
	// Pipe into a non-TTY → plain text automatically. --plain forces text
	// even in an interactive terminal.
	if opts.plain || !isTerminal(os.Stdout) {
		return runPlain(opts)
	}
	return runTUI(opts)
}

// runMetrics drives `git clarity metrics` — the weekly aggregates view.
//
// It reads exactly one snapshot and never polls. `git clarity` answers "is
// main green right now?"; this answers "are we getting better?", which is a
// question about trend and therefore about history, not about the last few
// seconds. Nothing here goes stale in a way a refresh would fix.
//
// Interactive on a terminal, because the deploy strip is a control and a tab
// bar you cannot press is not one. Piped or redirected, it prints once and
// exits — a strip is meaningless without a keyboard.
func runMetrics(args []string) error {
	fs := flag.NewFlagSet("metrics", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	deploy := fs.String("deploy", "", "open on one deploy flow by name")
	weeks := fs.Int("weeks", 12, "how many whole weeks to show")
	limit := fs.Int("limit", metricsCommitWindow, "commits to read; 0 for unlimited")
	cacheDir := fs.String("cache-dir", "", "override the cache directory")
	plain := fs.Bool("plain", false, "print once instead of opening the interactive view")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: git clarity metrics [--deploy <flow>] [--weeks N] [--plain]: %w", err)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unknown argument %q", fs.Arg(0))
	}
	if *weeks < 1 {
		return fmt.Errorf("--weeks must be at least 1, got %d", *weeks)
	}

	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	cfg, err := loadConfig(repoPath)
	if err != nil {
		return err
	}
	// A trend view wants history, and without a poll loop it can afford it:
	// the cost is paid once at startup rather than on every tick. 0 still
	// means unlimited, as it does on every other command.
	opts := rootOptions{limit: *limit, cacheDir: *cacheDir, deploy: *deploy}

	// The timeout bounds the FETCH, not the session. Reading the snapshot is
	// the only thing here that can hang; once it has landed the view is
	// entirely local and should stay open for as long as it is wanted.
	fetchCtx, cancelFetch := context.WithTimeout(context.Background(), metricsTimeout)
	defer cancelFetch()
	src, err := buildSource(fetchCtx, cfg, opts, repoPath)
	if err != nil {
		return err
	}

	view, err := firstFreshView(fetchCtx, lensFor(cfg, src).Views(fetchCtx))
	if err != nil {
		return err
	}
	view = core.TrimToWholeWeeks(view, *weeks)

	if *plain || !isTerminal(os.Stdout) {
		out, err := renderMetricsOnce(view, *deploy, metricsPlainWidth)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(os.Stdout, out)
		return err
	}
	// A context with no deadline: the interactive view was previously given
	// the fetch context, so it was killed mid-read sixty seconds after
	// launch with "context deadline exceeded".
	renderCtx, cancelRender := metricsRenderContext()
	defer cancelRender()
	return tui.RunWeekly(renderCtx, view, *deploy)
}

// metricsPlainWidth is the width the non-interactive render uses. Fixed
// rather than probed: piped output should be byte-identical run to run, and
// there is no terminal to measure when the destination is a file.
const metricsPlainWidth = 100

// metricsRenderContext returns the context the interactive view runs under.
//
// Separate from the fetch context, and deliberately without a deadline. The
// view holds a snapshot it has already read; nothing about sitting on screen
// can time out, and inheriting the fetch deadline killed the program while
// the user was reading it.
func metricsRenderContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// metricsTimeout bounds the one cold fetch this command makes. There is no
// cache-warm path here: unlike the live TUI it never paints a stale view
// first, so a hung fetch is visible as a wait rather than as stale numbers.
const metricsTimeout = 60 * time.Second

// metricsCommitWindow is the default commit window. Generous because twelve
// weeks of history is the point, and a one-shot read can afford what a
// five-second poll cannot.
const metricsCommitWindow = 2000

// firstFreshView takes the single snapshot the weekly view is built from.
func firstFreshView(ctx context.Context, views <-chan core.View) (core.View, error) {
	for {
		select {
		case v, ok := <-views:
			if !ok {
				return core.View{}, fmt.Errorf("source closed before emitting a view")
			}
			// Defensive rather than reachable today: this command builds a
			// bare Lens, and only CachedLens ever sets Stale. If it is ever
			// wrapped for a warm first paint, a one-shot read must still wait
			// for the real snapshot rather than report last week's numbers as
			// this week's.
			if v.Stale {
				continue
			}
			return v, nil
		case <-ctx.Done():
			return core.View{}, fmt.Errorf(
				"timed out after %s waiting for the first snapshot — "+
					"check the remote is reachable: %w", metricsTimeout, ctx.Err())
		}
	}
}

// renderMetricsOnce is the non-interactive path.
func renderMetricsOnce(view core.View, deploy string, width int) (string, error) {
	if deploy != "" {
		if _, ok := core.MatchFlow(view.Flows, deploy); !ok {
			return "", fmt.Errorf("no deploy flow named %q — this repo has: %s",
				deploy, strings.Join(core.FlowNames(view.Flows), ", "))
		}
	}
	// Resolved through the index, the way the interactive path does it.
	// MatchFlow is fold-insensitive and also matches a flow's targets, so the
	// string the user typed is often not the flow's name — and RenderWeekly
	// selects by exact name. Passing the raw query on meant a request that
	// validated fine rendered a different flow's numbers, silently.
	selected := ""
	if len(view.Flows) > 0 {
		selected = view.Flows[0].Name
	}
	if deploy != "" {
		i, _ := core.MatchFlow(view.Flows, deploy)
		selected = view.Flows[i].Name
	}
	return tui.RenderWeekly(view, selected, width) +
		truncationNotice(view, width), nil
}

// truncationNotice says when the commit window, rather than the repository's
// age, is what ended the history.
//
// The commit list already does this — "the bottom of a truncated scroll can't
// be mistaken for the start of the repository" — and the reasoning applies
// with more force here, because an aggregate that is short a few deploys is
// wrong in a way no reader can see.
func truncationNotice(view core.View, width int) string {
	if !view.Snapshot.Truncated {
		return ""
	}
	return tui.DimNotice(fmt.Sprintf(
		"  history ends at the %d-commit window, not at the first commit — pass --limit 0 for all of it",
		view.Snapshot.Limit), width) + "\n"
}

// runInit drives `git clarity init --github` — the interactive
// workflow + jobs picker that writes .ezcd.json. The actual flow lives
// in internal/initcmd so it can be tested with buffer-backed I/O; this
// function just wires the production stdin / stdout / gh client.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: git clarity init: %w", err)
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: git clarity init (it takes no arguments — there is nothing to ask)")
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	return initcmd.Run(initcmd.Options{RepoPath: repoPath, Out: os.Stdout})
}

// runOnboard prints the agent-onboarding prompt to stdout, so
// `git clarity onboard | wl-copy` (or a straight copy out of the terminal)
// hands the whole setup-and-backfill job to an AI agent.
func runOnboard(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: git clarity onboard (it prints a prompt for an AI agent; paste it into Claude Code or similar)")
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	fmt.Print(onboardcmd.Prompt(onboardcmd.GatherFacts(repoPath)))
	return nil
}

func parseRootArgs(args []string) (rootOptions, error) {
	fs := flag.NewFlagSet("clarity", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	plain := fs.Bool("plain", false, "force plain-text output (auto-enabled when stdout is not a tty)")
	showSHAs := fs.Bool("show-shas", false, "include short commit SHA per row")
	limit := fs.Int("limit", 100, "max commits to display; 0 means unlimited")
	cacheDir := fs.String("cache-dir", "", "directory for clarity's local caches; overrides $CLARITY_CACHE_DIR and the default .git/clarity")
	deploy := fs.String("deploy", "", "show one deploy flow by name; only meaningful in a repo with several")
	if err := fs.Parse(args); err != nil {
		return rootOptions{}, fmt.Errorf("usage: git clarity [--plain] [--show-shas] [--limit N] [--deploy <flow>] [--cache-dir <path>]: %w", err)
	}
	if fs.NArg() != 0 {
		return rootOptions{}, fmt.Errorf("unknown argument %q", fs.Arg(0))
	}
	return rootOptions{plain: *plain, showSHAs: *showSHAs, limit: *limit, cacheDir: *cacheDir, deploy: *deploy}, nil
}

// isTerminal reports whether f is a real character device (a terminal). False
// when stdout is being piped to another process, redirected to a file, or
// otherwise non-interactive — that's the trigger for auto-plain mode.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// loadConfig resolves the configuration every command operates on.
//
// The config of record is refs/clarity/config (fetched best-effort first,
// so offline answers with the last fetch): when it exists the repo counts
// as set up, its file applies, and any lingering legacy .ezcd.json is
// ignored rather than merged — two half-applied configs would be
// undebuggable. Without the ref, the legacy file is honoured so existing
// repos keep working through the deprecation window.
//
// The trunk comes from the repository itself (origin/HEAD, then main, then
// master — gitlog.DetectTrunk); only a legacy file's pinned branch
// overrides it. When nothing conventional resolves, cfg keeps the "main"
// default and the source pre-flight's error (with its hint) stays the
// ending.
func loadConfig(repoPath string) (config.Config, error) {
	_ = clarityrefs.FetchConfigRef(repoPath, "origin")
	data, ok, err := clarityrefs.ReadConfigFile(repoPath)
	if err != nil {
		return config.Config{}, err
	}
	var cfg config.Config
	if ok {
		if cfg, err = config.ParseClarityConfig(data); err != nil {
			return config.Config{}, err
		}
		cfg.SetUp = true
	} else if cfg, err = config.Load(repoPath); err != nil {
		return cfg, err
	}
	if !cfg.BranchPinned {
		if detected := gitlog.DetectTrunk(repoPath); detected != "" {
			cfg.Branch = detected
		}
	}
	return cfg, nil
}

func runTUI(opts rootOptions) error {
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The TUI comes up before any config is read or network touched:
	// everything slow or fallible happens in the orchestrator goroutine and
	// reports into the live frame — a phase line while it works, the welcome
	// pane when the repo isn't set up, the error in-frame when it cannot
	// start. Anything sitting between "typed the command" and "seeing
	// something" is where trust dies; nothing is allowed to sit there.
	views := make(chan core.View)
	startup := make(chan any, 8)
	choices := make(chan tui.WelcomeChoice, 1)
	go startDashboard(ctx, opts, repoPath, startup, choices, views)
	return tuiRendererFor(opts).RenderStartup(ctx, views, startup, choices)
}

// startDashboard is the TUI's startup orchestrator: config → (welcome →
// setup →) source pre-flight → views. Each step names itself on the
// startup channel before running, so a stall shows its stage instead of a
// bare "Loading"; each failure renders in-frame with its hint attached.
func startDashboard(ctx context.Context, opts rootOptions, repoPath string, startup chan<- any, choices <-chan tui.WelcomeChoice, views chan<- core.View) {
	send := func(msg any) bool {
		select {
		case startup <- msg:
			return true
		case <-ctx.Done():
			return false
		}
	}

	send(tui.StartupPhaseMsg("reading clarity config (refs/clarity/config)"))
	cfg, err := loadConfig(repoPath)
	if err != nil {
		send(tui.StartupErrorMsg{Err: err})
		return
	}

	// No config ref and no legacy file: the dashboard would render every
	// commit meaninglessly "before CI", so offer setup instead. "Exit" quits
	// inside the Model; only the setup choice arrives here.
	if !cfg.SetUp && !cfg.LegacyFile {
		// The wizard chain (may suggest the checked-out branch), not the
		// runtime one: this is a display suggestion a human reads, and on a
		// repo where nothing conventional resolves, naming the checkout
		// beats echoing the "main" default that just failed to detect.
		welcomeBranch := initcmd.DetectTrunk(repoPath)
		if welcomeBranch == "" {
			welcomeBranch = cfg.Branch
		}
		send(tui.WelcomeMsg{Branch: welcomeBranch})
		select {
		case <-choices:
		case <-ctx.Done():
			return
		}
		if err := initcmd.Run(initcmd.Options{RepoPath: repoPath, Out: io.Discard}); err != nil {
			send(tui.StartupErrorMsg{Err: err})
			return
		}
	}

	send(tui.StartupPhaseMsg(fmt.Sprintf("fetching %s from origin", cfg.Branch)))
	cacheDir := config.ResolveCacheDir(config.CacheDirSources{
		Flag:     opts.cacheDir,
		Env:      os.Getenv("CLARITY_CACHE_DIR"),
		RepoRoot: repoPath,
	})
	src, err := buildSource(ctx, cfg, opts, repoPath)
	if err != nil {
		send(tui.StartupErrorMsg{Err: err})
		return
	}

	send(tui.StartupPhaseMsg("reading events"))
	// CachedLens wraps the bare Lens so the TUI can paint a stale view
	// from <cacheDir>/snapshot-cache.json.gz immediately, then replace
	// it with the fresh fetch when the source's first emit lands. Plain
	// mode deliberately doesn't wrap (scripts/agents want fresh data).
	cf := cache.New(filepath.Join(cacheDir, "snapshot-cache.json.gz"))
	lens := cachedLensFor(cfg, src, cf)
	for v := range lens.Views(ctx) {
		select {
		case views <- v:
		case <-ctx.Done():
			return
		}
	}
}

// runPlain takes one snapshot from the source (which performs the initial
// fetch on first emit) and renders it as static text. Intended for piped
// agent / shell-script consumers.
//
// A 30s timeout protects against a hung fetch (network problems) by
// cancelling the source's context, which closes the views channel and
// makes the plain renderer's "source closed before emitting" error fire
// instead of wedging indefinitely.
func runPlain(opts rootOptions) error {
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	cfg, err := loadConfig(repoPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	src, err := buildSource(ctx, cfg, opts, repoPath)
	if err != nil {
		return err
	}
	lens := lensFor(cfg, src)
	// Limit is already applied by the source; passing 0 here means "don't
	// truncate further" inside RenderSnapshot.
	return plainRendererFor(opts).Render(ctx, lens.Views(ctx))
}

// checkTimeout bounds how long `check` waits for its first snapshot. The
// Source port emits on a channel and swallows its own read errors — a repo
// whose events ref cannot be read emits nothing at all rather than failing —
// so a clock is the only thing that separates "still fetching" from "never
// going to answer". Twenty seconds covers a cold fetch over a slow link
// without making a push feel hung; past it the answer is undetermined, which
// does not block.
const checkTimeout = 20 * time.Second

const checkHelp = `usage: git clarity check [--limit N] [--cache-dir <path>]

Exits 0 when CI is not red on the trunk, 1 when the newest commit that
resolved CI failed, and 0 with a note on stderr when it could not tell.
Prints nothing at all when the trunk is green, so it composes:

  git clarity check && git push

CI only. A failed deploy never blocks a push: it is not a failure another
commit makes worse, and it is per-flow besides.

There is no flag to pass the check. To push onto a red trunk, run ` + "`git push`" + `
on its own — if the break is yours to fix, that is the expected thing to do.
`

// runCheck drives `git clarity check` — the pre-push predicate.
//
// It reads one snapshot from the configured Source and classifies it. Three
// things it deliberately does not do:
//
//   - It does not go through the Lens. Groupings, lead times and flows are
//     irrelevant to one boolean, and CachedLens would hand back a stale
//     cached view first — gating a push on a cached answer is precisely the
//     bug this command would be blamed for.
//   - It does not print on green. The command is meant to be prefixed to
//     every push, and a tool that chatters on the happy path gets dropped
//     from the habit inside a week.
//   - It does not offer a way past itself. Dropping the `&&` is the way past,
//     and that is the only bypass that cannot be baked into a wrapper script.
func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 100, "max commits to read while looking for the newest resolved CI status")
	// --cache-dir is still accepted so existing pre-push hooks don't break,
	// but nothing in check uses a cache anymore: it was the retired GitHub
	// Actions source's run cache, and the events-ref read is cacheless by
	// design (a push gate answering from a cache would be answering about
	// the past).
	_ = fs.String("cache-dir", "", "accepted for compatibility; check no longer caches")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stdout, checkHelp)
			return nil
		}
		return checkFailed(fmt.Errorf("usage: git clarity check [--limit N] [--cache-dir <path>]: %w", err))
	}
	if fs.NArg() != 0 {
		return checkFailed(fmt.Errorf("unknown argument %q", fs.Arg(0)))
	}

	repoPath, err := repoRoot()
	if err != nil {
		return checkFailed(fmt.Errorf("not a git repository: %w", err))
	}
	cfg, err := loadConfig(repoPath)
	if err != nil {
		return checkFailed(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	result := checkTrunk(ctx, cfg, repoPath, *limit)
	if result.Report != "" {
		fmt.Fprintln(os.Stderr, strings.TrimRight(result.Report, "\n"))
	}
	if code := result.Outcome.ExitCode(); code != 0 {
		return exitError{code: code}
	}
	return nil
}

// checkTrunk reads one snapshot and classifies it, turning every failure into
// an undetermined answer rather than an error. Nothing that goes wrong while
// *finding out* whether the trunk is red is grounds for blocking a push —
// offline, an expired credential, a misconfigured source, a timeout: none of
// them is a broken build, and a check that blocked on any of them would be
// uninstalled the first time someone worked on a train.
func checkTrunk(ctx context.Context, cfg config.Config, repoPath string, limit int) checkcmd.Result {
	// Read through TrunkReader rather than the Source port, so a fetch that
	// failed comes back as "could not tell" instead of being quietly
	// answered from stale remote-tracking refs. That distinction is the
	// difference between a check that is useless offline and one that is
	// actively misleading.
	src, err := refsource.New(refsource.Options{
		RepoPath: repoPath,
		RepoName: filepath.Base(repoPath),
		Remote:   "origin",
		Branch:   cfg.Branch,
		Limit:    effectiveLimit(limit),
	})
	if err != nil {
		return checkcmd.Undetermined.Result(err.Error())
	}
	return checkcmd.Read(ctx, src, cfg.Branch, time.Now())
}

// checkFailed reports a check that could not run at all — a bad flag, not a
// git repository — and exits 2 rather than 1.
//
// The code matters: `check && git push` reads 1 as "the trunk is red", so a
// typo in a flag must not be able to impersonate a broken build. One meaning
// per exit code is the whole contract of a predicate.
func checkFailed(err error) error {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	return exitError{code: 2}
}

// exitError carries a process status out of a subcommand whose exit code is
// part of its interface and which has already written whatever the user needs
// to see. main exits with the code and prints nothing further.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// buildSource constructs the inbound Source adapter. There is one: the
// events-ref source. The factory shape survives its former second branch
// (the retired GitHub Actions adapter) because the config ref's
// clarity.source key is where future sources — another repository's ref,
// another backend — will dispatch from.
func buildSource(ctx context.Context, cfg config.Config, opts rootOptions, repoPath string) (core.Source, error) {
	src, err := refsource.New(refsource.Options{
		RepoPath: repoPath,
		RepoName: filepath.Base(repoPath),
		Remote:   "origin",
		Branch:   cfg.Branch,
		Limit:    effectiveLimit(opts.limit),
	})
	if err != nil {
		return nil, err
	}
	// Same pre-flight contract as the ghsource branch above: the Source port
	// swallows per-poll errors, so a branch that resolves nowhere would hang
	// the TUI on an empty channel, blank, forever. Fail here instead — and
	// since the usual cause is a repo whose trunk isn't the "main" default
	// and that has no .ezcd.json yet, say which branch probably is the trunk.
	if err := src.Validate(ctx); err != nil {
		return nil, fmt.Errorf("events source: %w%s", err, branchHint(cfg, repoPath))
	}
	return src, nil
}

// branchHint names the likely fix when the configured branch doesn't
// resolve. Detection already tried origin/HEAD and the conventional names,
// so landing here means the repo's trunk is unconventional AND git's own
// record of the remote default is missing or stale — which is why the hint
// is a git repair, not a clarity config: `git remote set-head` fixes trunk
// detection for every tool that reads origin/HEAD, this one included.
func branchHint(cfg config.Config, repoPath string) string {
	detected := initcmd.DetectTrunk(repoPath)
	if detected == "" || detected == cfg.Branch {
		return "\nclarity watches the repository's trunk (origin/HEAD, then main, then master) — none of those resolved here"
	}
	return fmt.Sprintf("\nthis repository's trunk looks like %q — run `git remote set-head origin --auto` so the repo records its default branch, and clarity (like every origin/HEAD reader) will find it",
		detected)
}

// effectiveLimit maps the CLI's "0 = unlimited" convention onto the source's
// Limit field, which would otherwise reset 0 back to its own default of 50.
func effectiveLimit(cliLimit int) int {
	if cliLimit <= 0 {
		return math.MaxInt32
	}
	return cliLimit
}

func runReport(args []string) error {
	if isBatchInvocation(args) {
		return runReportBatch(args)
	}
	// Two grammars under one command, dispatched on the first non-flag token.
	// Unambiguous, because `affected` is not a stage — and candidacy has no
	// status to report, so forcing it into the stage/status shape would need a
	// filler word that means nothing.
	return dispatchReport(os.Stdout, args, report.Run, report.RunScope)
}

// dispatchReport routes an invocation to the grammar it uses. Separated from
// runReport so the routing can be tested with injected writers — a predicate
// that is correct but never consulted is the same as not having one.
func dispatchReport(
	out io.Writer,
	args []string,
	writeEvent func(report.Options) (string, error),
	writeScope func(report.ScopeOptions) (string, error),
) error {
	if isScopeInvocation(args) {
		return runReportScopeTo(out, args, writeScope)
	}
	return runReportTo(out, args, writeEvent)
}

// isScopeInvocation reports whether these args use the candidacy grammar,
// looking past the flags for the verb.
func isScopeInvocation(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--sha", "--at", "-sha", "-at":
			i++ // skip the flag's value
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a == "affected" || a == "unaffected"
	}
	return false
}

func runReportScopeTo(out io.Writer, args []string, write func(report.ScopeOptions) (string, error)) error {
	opts, err := parseScopeArgs(args)
	if err != nil {
		return err
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	opts.RepoPath = repoPath
	opts.Remote = "origin"

	if err := report.ValidateScope(opts); err != nil {
		return err
	}
	if err := checkDeclaredTarget(repoPath, opts.Target); err != nil {
		return err
	}

	opts, err = report.ResolveScope(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "running: %s\n", report.ScopeCommandLine(opts))

	sha, err := write(opts)
	if err != nil {
		return report.ScopeFailureError(opts, err)
	}
	short := sha
	if len(short) > 8 {
		short = short[:8]
	}
	verb := "unaffected"
	if opts.Affected {
		verb = "affected"
	}
	fmt.Fprintf(out, "wrote scope: %s %s %s\n", short, verb, opts.Target)
	return nil
}

const scopeUsage = "usage: git clarity report [--sha <sha>] [--at <rfc3339>] affected|unaffected <target>"

func parseScopeArgs(args []string) (report.ScopeOptions, error) {
	fs := flag.NewFlagSet("report scope", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sha := fs.String("sha", "", "explicit commit SHA (overrides GITHUB_SHA / CI_COMMIT_SHA / HEAD)")
	at := fs.String("at", "", "explicit timestamp in RFC3339 (overrides time.Now())")
	if err := fs.Parse(args); err != nil {
		return report.ScopeOptions{}, fmt.Errorf("%s: %w", scopeUsage, err)
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return report.ScopeOptions{}, fmt.Errorf("%s", scopeUsage)
	}
	// An empty target argument is what `report affected "$TARGET"` produces
	// with TARGET unset. Candidacy needs a target to be about, so this is
	// caught here rather than allowed to become a record naming nothing.
	if rest[1] == "" {
		return report.ScopeOptions{}, fmt.Errorf("target is empty — %s", scopeUsage)
	}
	opts := report.ScopeOptions{Target: rest[1], SHA: *sha, Affected: rest[0] == "affected"}
	if *at != "" {
		t, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			return report.ScopeOptions{}, fmt.Errorf("invalid --at timestamp (want RFC3339, e.g. 2006-01-02T15:04:05Z): %w", err)
		}
		opts.Time = t
	}
	return opts, nil
}

// runReportTo is runReport with its output sink and the write itself injected,
// so the surrounding reporting behaviour can be tested without a remote.
//
// It echoes the fully-resolved invocation before writing. A report that fails
// leaves the stage unrecorded, and nothing retries it later — the commit keeps
// showing as in-flight in the TUI until someone writes the event by hand. The
// echo puts the command to do that in the log up front, which matters because
// a step that is killed or times out never reaches the failure path at all.
// On failure the same command is repeated in the error, where anyone reading
// a red log is already looking.
func runReportTo(out io.Writer, args []string, write func(report.Options) (string, error)) error {
	opts, err := parseReportArgs(args)
	if err != nil {
		return err
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	opts.RepoPath = repoPath
	opts.Remote = "origin"

	// Validate before the echo. The echoed line exists so a failed report can
	// be re-run verbatim, and echoing an invocation that is itself the mistake
	// would invite exactly that.
	if err := report.Validate(opts); err != nil {
		return err
	}
	if err := checkDeclaredTarget(repoPath, opts.Target); err != nil {
		return err
	}

	opts, err = report.Resolve(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "running: %s\n", report.CommandLine(opts))

	sha, err := write(opts)
	if err != nil {
		return report.FailureError(opts, err)
	}
	short := sha
	if len(short) > 8 {
		short = short[:8]
	}
	fmt.Fprintf(out, "wrote event: %s %s %s\n", short, opts.Stage, opts.Status)
	return nil
}

// runReportBatch reads JSON Lines from stdin and writes the whole batch in a
// single commit + push. Each line is one event: {"sha","at","stage","status"}.
// The amortised round-trip is the point — a 200-event backfill becomes ~one
// push instead of 200.
func runReportBatch(args []string) error {
	fs := flag.NewFlagSet("report --batch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	batch := fs.Bool("batch", false, "read events from stdin as JSON Lines")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: git clarity report --batch < events.jsonl: %w", err)
	}
	if !*batch || len(fs.Args()) != 0 {
		return fmt.Errorf("usage: git clarity report --batch < events.jsonl")
	}
	repoPath, err := repoRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	events, err := readBatchEvents(os.Stdin)
	if err != nil {
		return err
	}
	if err := report.RunBatch(report.BatchOptions{
		RepoPath: repoPath,
		Remote:   "origin",
	}, events); err != nil {
		return err
	}
	fmt.Printf("wrote %d events\n", len(events))
	return nil
}

func isBatchInvocation(args []string) bool {
	for _, a := range args {
		if a == "--batch" {
			return true
		}
	}
	return false
}

func readBatchEvents(r io.Reader) ([]report.BatchEvent, error) {
	var events []report.BatchEvent
	sc := bufio.NewScanner(r)
	// Bump the scanner buffer in case future event payloads grow past the
	// default 64KB line limit; current backfill lines are ~150 bytes.
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ev, err := parseBatchLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		// Carry the line number so a validation failure downstream points at
		// the same line the parse errors above do, rather than at a slice
		// index that blank lines have already shifted.
		ev.Line = lineNum
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return events, nil
}

func parseBatchLine(line string) (report.BatchEvent, error) {
	var raw struct {
		SHA    string `json:"sha"`
		At     string `json:"at"`
		Stage  string `json:"stage"`
		Status string `json:"status"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return report.BatchEvent{}, fmt.Errorf("invalid JSON: %w", err)
	}
	t, err := time.Parse(time.RFC3339, raw.At)
	if err != nil {
		return report.BatchEvent{}, fmt.Errorf("invalid 'at' (want RFC3339): %w", err)
	}
	return report.BatchEvent{
		SHA:    raw.SHA,
		Time:   t,
		Stage:  raw.Stage,
		Status: raw.Status,
		Target: raw.Target,
	}, nil
}

// parseReportArgs parses CLI args for `git clarity report` into report.Options.
// Flags must come before the positional <stage> <status> args. `--sha` and
// `--at` are migration overrides: --sha takes precedence over GITHUB_SHA /
// CI_COMMIT_SHA / HEAD, and --at (RFC3339) replaces the default time.Now().
// reportUsage is the one-line synopsis shown for every malformed invocation.
// The target is listed so the argument is discoverable from the error rather
// than only from the docs.
const reportUsage = "usage: git clarity report [--sha <sha>] [--at <rfc3339>] <stage> <status> [<target>]"

func parseReportArgs(args []string) (report.Options, error) {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sha := fs.String("sha", "", "explicit commit SHA (overrides GITHUB_SHA / CI_COMMIT_SHA / HEAD)")
	at := fs.String("at", "", "explicit event timestamp in RFC3339 (overrides time.Now())")
	if err := fs.Parse(args); err != nil {
		return report.Options{}, fmt.Errorf("%s: %w", reportUsage, err)
	}
	rest := fs.Args()
	if len(rest) < 2 || len(rest) > 3 {
		return report.Options{}, fmt.Errorf("%s", reportUsage)
	}
	opts := report.Options{
		Stage:  rest[0],
		Status: rest[1],
		SHA:    *sha,
	}
	// The optional third positional names the deployable. Positional rather
	// than a flag because it is a fixed-arity part of what is being reported,
	// and `report deploy passed ios` is what a pipeline step should read like.
	if len(rest) == 3 {
		// An explicitly empty argument is not the same as no argument. It is
		// what `report deploy passed "$TARGET"` produces with TARGET unset,
		// and silently recording an untargeted deploy there would put the
		// event in the wrong flow with a zero exit code.
		if rest[2] == "" {
			return report.Options{}, fmt.Errorf(
				"target is empty — omit the argument entirely to report the untargeted deploy")
		}
		opts.Target = rest[2]
	}
	if *at != "" {
		t, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			return report.Options{}, fmt.Errorf("invalid --at timestamp (want RFC3339, e.g. 2006-01-02T15:04:05Z): %w", err)
		}
		opts.Time = t
	}
	return opts, nil
}

func repoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Env = gitenv.Clean()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// lensFor and cachedLensFor are the single place configuration is handed to
// the derivation layer. They exist as named functions rather than inline
// construction so the wiring is testable: `clarity.leadTime` once shipped
// doing nothing because a configured value never reached the layer that
// consumed it, and this is exactly the seam where that happens.
func lensFor(cfg config.Config, src core.Source) *core.Lens {
	return core.NewLens(src, cfg.LeadTimeMode(), cfg.Deploys())
}

func cachedLensFor(cfg config.Config, src core.Source, cf *cache.File) *core.CachedLens {
	mode, flows := cfg.LeadTimeMode(), cfg.Deploys()
	return core.NewCachedLens(core.NewLens(src, mode, flows), cf, mode, flows)
}

// checkDeclaredTarget refuses a target the repo's config doesn't declare.
//
// Declaring flows is what turns the config into an expectation, and this is
// where that expectation pays: a typo fails the pipeline that made it, loudly,
// instead of appearing days later as a mysterious extra tab that nobody
// deploys to. Events are append-only, so the alternative is a stray flow
// nobody can remove.
//
// A repo that declares nothing keeps the open vocabulary by design — discovery
// is the zero-setup path and must not require any config to work.
//
// Through loadConfig, not the bare legacy file: the declarations live in
// refs/clarity/config now, and a CI clone only sees those after the fetch
// loadConfig performs — skipping it would silently disable enforcement in
// exactly the place deploys are reported from.
func checkDeclaredTarget(repoPath, target string) error {
	if target == "" {
		return nil
	}
	cfg, err := loadConfig(repoPath)
	if err != nil {
		return err
	}
	flows := cfg.Deploys()
	if len(flows) == 0 {
		return nil
	}
	for _, f := range flows {
		if f.Claims(target) {
			return nil
		}
	}

	declared := make([]string, 0, len(flows))
	for _, f := range flows {
		declared = append(declared, f.Name)
	}
	return fmt.Errorf("target %q is not declared in the clarity config — this repo declares: %s\n\n"+
		"  Add a [deploy %q] section to the config (refs/clarity/config), or fix the\n"+
		"  typo. Events are append-only, so an undeclared target would show up as a\n"+
		"  flow nobody can remove.",
		target, strings.Join(declared, ", "), target)
}

// tuiRendererFor and plainRendererFor are the single place root flags are
// handed to a renderer. Named functions rather than inline construction so the
// wiring is testable: a flag that parses correctly and never reaches the
// renderer is inert, and looks like a working feature from every other angle.
func tuiRendererFor(opts rootOptions) *tui.Renderer {
	return tui.NewRenderer().WithFlow(opts.deploy)
}

func plainRendererFor(opts rootOptions) *plain.Renderer {
	return plain.NewRenderer(plain.Options{ShowSHAs: opts.showSHAs, Flow: opts.deploy})
}
