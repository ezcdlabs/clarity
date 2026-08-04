package core

import (
	"context"
	"encoding/json"

	"github.com/ezcdlabs/clarity/internal/cache"
)

// ViewProducer is the small port CachedLens wraps. *Lens implements it;
// other future composites (e.g. a Lens chained with a metrics decorator)
// can too. Lets CachedLens compose without depending on the concrete
// *Lens type.
type ViewProducer interface {
	Views(ctx context.Context) <-chan View
}

// CachedLens decorates a ViewProducer with stale-while-revalidate
// behaviour: on Views(ctx) it first emits a cached View marked Stale=true
// (when a usable cache file exists), then forwards every fresh View from
// the underlying producer with Stale=false, persisting each fresh
// Snapshot back to the cache as a side effect.
//
// Used by the TUI for instant first paint. Plain mode wraps a bare Lens
// instead — scripts and agents want fresh data, and the small "warm the
// cache for the next TUI run" benefit isn't worth complicating plain
// mode's one-shot exit semantics with cache writes today. If we want
// that later, it'll be a thin write-only decorator alongside this one.
type CachedLens struct {
	inner ViewProducer
	cache *cache.File
	mode  LeadTimeMode
}

// NewCachedLens returns a CachedLens wrapping inner and persisting
// snapshots to cf. The cache file may be missing on first run — CachedLens
// simply skips the stale emission in that case.
// The mode must match the one the inner ViewProducer uses, so the stale view
// painted at startup measures lead time the same way the fresh view that
// replaces it will. An empty mode means DefaultLeadTimeMode.
func NewCachedLens(inner ViewProducer, cf *cache.File, mode LeadTimeMode) *CachedLens {
	if mode == "" {
		mode = DefaultLeadTimeMode
	}
	return &CachedLens{inner: inner, cache: cf, mode: mode}
}

// Views streams Views to the caller. Emissions happen in this order:
//
//  1. One Stale=true View, if the cache file exists and parses. Missing
//     or corrupted cache silently falls through to step 2 — startup
//     stays robust on broken state.
//  2. Each fresh View from the inner producer, with Stale=false. Every
//     such View's Snapshot is written to the cache (best-effort —
//     write errors don't block the stream).
//
// Closes when the inner producer's channel closes (i.e. when ctx is
// cancelled by the caller and that propagates down to the Source).
func (c *CachedLens) Views(ctx context.Context) <-chan View {
	out := make(chan View)
	go func() {
		defer close(out)

		if stale, ok := c.readStale(); ok {
			select {
			case out <- stale:
			case <-ctx.Done():
				return
			}
		}

		for v := range c.inner.Views(ctx) {
			c.writeFresh(v.Snapshot)
			v.Stale = false
			select {
			case out <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// readStale returns a Stale=true View derived from the cache file, or
// (zero, false) when the cache is absent / corrupted. Corruption is
// non-fatal — the caller should still take the fresh path.
func (c *CachedLens) readStale() (View, bool) {
	data, exists, err := c.cache.Read()
	if err != nil || !exists {
		return View{}, false
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return View{}, false
	}
	v := DeriveView(snap, c.mode)
	v.Stale = true
	return v, true
}

// writeFresh persists the given Snapshot to the cache. Errors are
// swallowed — failing to keep the cache warm shouldn't break the live
// pipeline, and the next fresh emit will overwrite anyway.
func (c *CachedLens) writeFresh(snap Snapshot) {
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	_ = c.cache.Write(data)
}
