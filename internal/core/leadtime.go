package core

import (
	"fmt"
	"strings"
	"time"
)

// LeadTimeMode selects which commits contribute a lead time, and what that
// lead time is measured from.
//
// The choice exists because "lead time" answers two different questions
// depending on where you start it, and which one a team wants depends on what
// they intend to change. Measuring from the commit answers "how long from
// writing code to it running in production" — DORA's lead time for changes,
// which deliberately includes time the work sat unpushed, because that is
// real delay in the value stream. Measuring from the first pipeline event
// answers "how long does our pipeline take once it has the code", which is
// what you want if the number is meant to drive pipeline work rather than
// commentary on when people push.
//
// The modes are ordered by how much developer behaviour they let into the
// number: LeadAll the most, LeadPipeline the least.
type LeadTimeMode string

const (
	// LeadAll measures every commit from its authoring time, including
	// commits with no pipeline events of their own — they inherit the deploy
	// that shipped them. This is clarity's historical behaviour and remains
	// the default, so upgrading doesn't silently move anyone's numbers.
	//
	// Its weakness is that CI typically runs on the pushed head rather than
	// every commit in the push, so a developer who commits five times and
	// pushes once contributes five samples that all carry however long the
	// work sat locally. The number ends up as much a measure of commit and
	// push habits as of the pipeline.
	LeadAll LeadTimeMode = "all"

	// LeadReported measures only commits that carry at least one pipeline
	// event, still from their authoring time. Commits swept along by someone
	// else's deploy stop contributing.
	//
	// This removes the multiplier — one sample per pushed batch instead of
	// one per commit — but not the magnitude: the surviving commit is still
	// timed from when it was authored, so a Friday commit pushed on Monday
	// still puts a weekend into the average. Worth choosing if you want the
	// DORA definition kept intact and only the double-counting removed.
	LeadReported LeadTimeMode = "reported"

	// LeadPipeline measures only commits that carry at least one pipeline
	// event, from the earliest of those events rather than from authoring.
	//
	// This is the mode to pick if the number is meant to describe the
	// pipeline: time spent unpushed, in review, or otherwise before CI took
	// the code stops counting entirely. Three things follow from that, all
	// deliberate. It is no longer DORA lead time for changes and shouldn't be
	// reported as such. Anything before the first event is invisible,
	// including runner queueing, which is genuine pipeline performance. And
	// the number depends on teams reporting `ci started` — a team reporting
	// only the passed events measures a much shorter interval, because the
	// start moves to the end of CI.
	LeadPipeline LeadTimeMode = "pipeline"
)

// DefaultLeadTimeMode is what an unset configuration means.
const DefaultLeadTimeMode = LeadAll

// ParseLeadTimeMode converts a configured string into a LeadTimeMode. Empty
// means unset and yields the default. Matching is case-insensitive so a
// hand-edited config file isn't rejected over capitalisation.
func ParseLeadTimeMode(s string) (LeadTimeMode, error) {
	switch LeadTimeMode(strings.ToLower(strings.TrimSpace(s))) {
	case "":
		return DefaultLeadTimeMode, nil
	case LeadAll:
		return LeadAll, nil
	case LeadReported:
		return LeadReported, nil
	case LeadPipeline:
		return LeadPipeline, nil
	default:
		return "", fmt.Errorf(
			"unknown lead time mode %q — must be one of %q, %q or %q",
			s, LeadAll, LeadReported, LeadPipeline)
	}
}

// leadStarts computes, per commit, the instant its lead time is measured
// from. A zero value means the commit has no lead time under this mode: it
// renders without one and contributes nothing to the weekly average.
func leadStarts(commits []CommitView, mode LeadTimeMode) []time.Time {
	out := make([]time.Time, len(commits))
	for i, c := range commits {
		switch mode {
		case LeadReported:
			if len(c.Events) > 0 {
				out[i] = c.Time
			}
		case LeadPipeline:
			out[i] = earliestEventTime(c.Events)
		default: // LeadAll
			out[i] = c.Time
		}
	}
	return out
}

// earliestEventTime returns the oldest event timestamp, or the zero time when
// there are no events. Scans rather than taking the first entry: event slices
// reach the core from several adapters and not all of them promise an order.
func earliestEventTime(events []Event) time.Time {
	var earliest time.Time
	for _, e := range events {
		if e.Time.IsZero() {
			continue
		}
		if earliest.IsZero() || e.Time.Before(earliest) {
			earliest = e.Time
		}
	}
	return earliest
}
