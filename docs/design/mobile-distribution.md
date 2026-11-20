# Mobile distribution

The phone apps ship through the repository's own GitHub releases — the same
releases the CLI cuts, because the Go core is bound into both apps and every
release therefore genuinely changes them. There is no store, no account with a
platform, and no second release cadence to keep in step with the first.

## Release assets

A reusable workflow (`mobile-assets.yml`) runs after the release job and
attaches the apps to the release it just cut, built at the tag so the assets
are exactly the commit the release names. It is also dispatchable by hand
against any tag, which is both the backfill path (releases that predate the
workflow) and the recovery path (a transient mobile build failure leaves a
usable CLI release rather than no release; re-run the dispatch to attach the
missing assets).

Assets carry **stable names** — `clarity-android.apk`, with a
`clarity-android.apk.sha256` sidecar — because the updater downloads
`releases/latest/download/<name>`, and a name that embedded the version would
make every client guess it before it could fetch it.

## Android: signing

The APK is signed with one long-lived release key, held in Actions secrets
with its durable home offline. The key is the whole ballgame: Android enforces
signature continuity, so an update installs *as an update* only when it is
signed by the installed app's key. Publishing even one release under a
different key would strand every phone that installed it — uninstall/reinstall
is the only way out, taking local state with it. A local build without the CI
signing variables falls back to the debug key and versionCode 1, so nothing
built on a laptop can masquerade as a published release, and a dev install
always offers to "update" to a real one.

`versionName` is the tag's semver; `versionCode` is
`major·10⁶ + minor·10³ + patch` — monotonic for Conventional-Commits-derived
versions (minor and patch stay under 1000) and recoverable from any APK by
inspection.

## Android: the self-updater

The app checks its own releases feed and installs updates itself, because its
only distribution channel is a GitHub release page nobody revisits. The labour
is split on the usual boundary — **decisions in Go, platform by nature in
Kotlin**:

- `mobile/internal/update` decides: one anonymous request to the GitHub
  releases API (generic User-Agent, no user data; the unauthenticated rate
  limit is generous for a check once per process), strict semver comparison —
  an update is offered only for a *strictly newer* release, so a dev build
  reading `0.0.0-dev` updates to anything real and an equal version offers
  nothing. A malformed release tag is an error, not a quiet "no update":
  silently never updating again is the worst way to learn of a
  release-process change. A release without the APK asset is quietly no
  update — the asset job may have failed, and the app asks again next open.
- The platform downloads the file, verifies the sha256 sidecar when one was
  published (a corruption guard, not the security boundary — that is the
  installer's signature check), and commits a `PackageInstaller` session. The
  system's own confirmation dialog does the installing; nothing can install
  silently.

The check runs **once per process**, on app open, and failure is silent: it is
an unasked-for courtesy that needs the network, and an error bar about it
would outweigh it. When an update exists, the repository switcher gains a row
— "Update to 1.4.0" — which is also where download progress and a retry live.
Being current shows nothing: "up to date" is a row spent saying nothing. An
install in flight belongs to the process, not the sheet (`AppUpdater`), so
closing the switcher mid-download loses nothing.

**The Play Store trade is recorded and accepted.** `REQUEST_INSTALL_PACKAGES`
for self-updating is forbidden on Google Play, so this design forecloses a
Play listing. Shipping through releases was chosen anyway: the audience
already has the repository, and a store adds an account, a review queue and a
second signing identity for an app whose whole feed is `git fetch`. Users who
prefer a store-shaped updater can point Obtainium at the repository — it reads
the same releases.

## iOS

iOS cannot self-update — there is no side-loadable installer to hand a file
to. Distribution goes through AltStore instead: an unsigned IPA attached to
each release (AltStore re-signs it with the user's own Apple ID) and a source
JSON that AltStore watches for updates. Not implemented yet; this section is
the placeholder the implementation will replace.
