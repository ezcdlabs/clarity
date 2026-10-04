# Handoff: Git Clarity Android, app flow and polish

## Overview
This redesigns every stage of the Android app in `mobile/apps/android` (ezcdlabs/clarity). It covers the first open with no repos, connecting a repo (device key, SSH address, host-key trust, permission denied, first fetch), a single repo, a monorepo with several deploy flows, switching repos, the per-repo menu, renaming, removing, and names derived from the URL path.

The repo feed keeps the TUI's structure so users can move between the two. Polish comes from a type scale, spacing, real icons and native Material 3 controls. Metrics are out of scope.

## About the design files
`Clarity Android.dc.html` (open it in a browser, with `support.js` next to it) is a **design reference built in HTML**, not code to ship. Recreate it in the existing Jetpack Compose app using its current patterns: the `Ink` palette in `ui/Theme.kt`, `Sheet`, `StatusGlyph`, the `View` proto and `ClarityModel`. Use native M3 components wherever this README names one.

The file is a canvas of numbered rows. Each screen has an id, and most have a dark and a light copy side by side:
- **Row 0 (0a–0e):** today's app, recreated as a baseline. Don't implement.
- **Row 1 (1a–1i):** the directions explored. For context only. The chosen pieces are in rows 2–4.
- **Row 2 (2a–2j): the main flow. Implement this.**
- **Row 3 (3a–3e): the repo menu, rename, remove, and the naming rule. Implement this.**
- **Row 4 (4a–4d):** icon glyphs instead of text characters. **Still an open choice**; see "Open decisions".

## Fidelity
High fidelity. Colours, type, spacing and radii are final. The phone frames are 360×780dp.

## Global changes
- **Navigation:** replace the `HorizontalPager` (repos list ↔ repo) with a single repo screen. The repo name in the top bar opens a `ModalBottomSheet` switcher (2i, 3d). With no repos, the app opens on the empty state (2a).
- **Overlays:** connect a repo becomes one full-screen flow, opened from the empty state and from "Add" in the switcher. The separate Device key screen is reached from the switcher's "Device key" row and reuses the key card component.
- **Icons:** Material Symbols Rounded for the chrome (close, more_vert, expand_more, key, content_copy, alt_route, refresh, add, add_link, edit, delete, lock, error, fingerprint, terminal, check_circle). This replaces the text glyphs `‹` and `≡`.
- **Edge-to-edge** with `imePadding()` on every screen that has a text field (see 2j and 3b).

## Design tokens
### Colour (unchanged roles from `Theme.kt`)
| Role | Dark | Light |
|---|---|---|
| bg (sheet, reading surface, onboarding pages) | #0E0E0E | #FCFCFC |
| surface (chrome, cards, bottom sheets, dialogs) | #1A1A1A | #EDEDED |
| line (rules, outlines, selected row in sheet) | #2B2B2B | #DCDCDC |
| text | #E6E6E6 | #061732 |
| dim | #8C8C8C | #6B6B6B |
| red | #E06C75 | #C0392B |
| green | #98C379 | #2E7D32 |
| yellow (CI Passed band) | #F7C421 | #8A6D00 |
| blue (primary, Deployed band) | #6AA2FF | #1565C0 |
| errorBg | #2A1416 | #FBE9E9 |

Derived colours:
- **onPrimary:** `bg`, so dark text on blue in dark mode and white on blue in light. This fixes today's purple leak from the M3 defaults.
- **Tonal button / selected nav fill:** blue at 16% (dark) or `#1565C0` at 10% (light).
- **Disabled filled button:** text colour at 12% (dark) or `#061732` at 8% (light). Its label is `dim`.
- **Scrim:** black at 60% (dark) or 32% (light).
- **Dropdown menu container:** #2B2B2C (dark) or #FFFFFF (light). The menu divider is #3A3A3B (dark) or #E4E4E4 (light).
- **Snackbar:** inverse colours, `text` as background and `bg` as text.

### Typography
- **UI:** Roboto Flex. On device the system Roboto is an acceptable fallback.
- **Monospace:** JetBrains Mono for anything copyable or code-like: keys, fingerprints, URLs, branch names, lead times, and status glyphs if text glyphs are kept.

| Token | Size / line height | Weight | Tracking |
|---|---|---|---|
| Display (empty-state headline) | 34/40 | 650 | -0.8 |
| Page title (Connect a repository) | 28/34 | 650 | -0.6 |
| Dialog title | 22/28 | 600 | -0.3 |
| Top-bar repo title / sheet title | 20 | 600 (namespace prefix 400, dim) | -0.3 |
| Small app-bar title | 17–18 | 600 | -0.2 |
| Body | 15/22 (onboarding), 14/20 (secondary copy) | 400 | 0 |
| Card title | 15–16 | 600 | 0 |
| Supporting text | 12.5–13.5 / 17–19 | 400 | 0 |
| Overline | 11/14 caps | 600 | +1.2 |
| Mono (key) | 11.5–12 / 17–18 | 400 | 0 |
| Mono (meta) | 11–11.5 | 400 | 0 |
| Button | 15 (52dp tall), 13.5–14 (36–40dp tall) | 600 | 0 |

### Spacing, radius and elevation
- **Page margin:** 20dp (24dp in dialogs and the empty state). Spacing is on an 8dp rhythm (4, 8, 12, 16, 20, 24, 28).
- **Top app bar:** 64dp, with 48dp icon touch targets and a 4dp end inset.
- **Radii:** phone sheet top corners 20; bottom sheet 28; dialog 28; cards 16–20; text fields 12; inner key block 12; switcher rows 16; drawer items 28. Primary button is fully rounded (52dp tall, radius 26).
- **Shadows:** menu `0 6 24 rgba(0,0,0,.45)` in dark. Otherwise use M3 defaults.

## Screens

### 2a: First open, no repos
- **Background:** `bg` throughout.
- **App bar:** the logo mark at 28dp (crop `splash_mark` to its circle), then "Git Clarity" 17/600, with `more_vert` (Device key) at the end.
- **Body:** vertically centred with 24dp horizontal padding.
  - Overline "No repositories yet".
  - Display "Is main green?", with 12dp above.
  - Body copy (dim, 15/22): "Connect a repository to see what just landed, what passed CI and what's live in production. It's the same view as `git clarity` in your terminal."
  - A three-line legend with 14dp gaps: glyph (16dp column), band label (76dp column, 13/600 in band colour), dim description: HEAD "what just landed", CI Passed "green, waiting to ship", Deployed "live, with lead times".
- **Bottom:** filled button "Connect a repository" with `add_link`, 52dp tall. Below it, caption "Any git remote over SSH. Read-only." (12.5, dim, centred, 12dp gap).

### 2b: Connect, first run
- **App bar:** `close`. Title "Connect a repository" (28/650), then the subtitle "Clarity reads it over SSH, like `git fetch`."
- **SSH clone address field:** M3 `OutlinedTextField`, radius 12, focused 2dp blue. Keyboard type Uri, autocorrect off, IME action Go.
- **Branch field:** leading `alt_route`, value `main` in mono, trailing "branch" label. Blank means main, as today.
- **Key card:** `surface`, radius 20, padding 16.
  - Header: `key` icon, "This device's key" (15/600), and an `expand_less` toggle.
  - Copy: "Add it on your git host first, either as a read-only deploy key on the repository or under your account's SSH keys."
  - The key in a `bg` block, radius 12, padding 12, mono 11.5/17, breaking anywhere.
  - Footer: the fingerprint `SHA256:…` (mono 11, dim) and a tonal "Copy key" button (36dp).
- **Key card rule:** open by default until any repo has connected successfully with this key. After that it starts collapsed as a single row (see 1e): "Connects with this device's key", with `ed25519 · SHA256:…` underneath.
- **Primary button:** "Connect". Disabled while the address is blank.

### 2c: Connecting
- The fields become read-only (dim text, `line` outline). The close icon stays enabled and cancels.
- **Progress card:** `surface`, radius 20. Overline with the host name, then step rows (glyph column 16dp, label 14, optional elapsed time in mono 11.5):
  - ✓ Reached host
  - ✓ Signed in with device key
  - ◌ (spinner) Reading `main` and events
  - · Building the view

  These steps should map to real stages reported by the core. If the core can't report them, show only the spinner and the "Connecting…" button.
- **Button:** disabled, with an inline spinner and "Connecting…".

### 2d: First connection, verify host key
- An M3 `AlertDialog` over 2c, with the `fingerprint` icon (blue).
- **Title:** "Trust this host?"
- **Body:** "This is the first connection to `git.acme.dev`. Check that this fingerprint matches the one your host publishes."
- **Fingerprint block:** `bg`, radius 12. Key type (ED25519, mono 10.5 caps), then `SHA256:…` (mono 12).
- **Footnote:** "Clarity remembers it. If it ever changes, you'll be asked again."
- **Actions:** text button "Cancel" and filled button "Trust and continue".
- If the host key changes later, use the same dialog but red, with "Host key changed". This variant isn't drawn.

### 2e: Permission denied (key not authorised)
- **Address field** in the error state: 2dp red outline and a trailing filled `error` icon.
- **Error card:** `errorBg`, radius 20.
  - `lock` icon (red) and title "The host didn't accept this device's key".
  - Body: "Add the key below to the repository's deploy keys (read access is enough), then try again."
  - A "Show git output" link with the `terminal` icon. It expands to the raw stderr in mono, e.g. `Permission denied (publickey).`
- **Key card:** forced open, without the collapse toggle.
- **Button:** "Try again" with the `refresh` icon.

### 2j: Permission denied with the keyboard up (applies to 2b too)
- With the IME open, the large title collapses into a 64dp app bar titled "Connect a repository" with a bottom `line` divider.
- The error card shrinks to red supporting text under the field.
- The content scrolls, with the key card below the fold.
- The primary button docks above the keyboard (48dp tall, top `line` divider).

### 2f: Success, first fetch
- The repo screen, in its empty state:
  - The strip shows `ci: ·  deploy: ·` and "fetching…" on the right.
  - A 4dp `LinearProgressIndicator` sits on the chrome, under the strip.
  - The sheet shows all three band headers with skeleton bars (in `surface`).
  - Note: "Reading the last 100 commits on `main` for the first time. Later refreshes only fetch what changed."
- **Snackbar:** "Connected to web-platform" with `check_circle`.

### 2g: Repository, single flow (the main screen)
- **Status bar and chrome:** `surface`.
- **App bar (64dp):**
  - Left, two lines. Title `namespace/` (400, dim) then `name` (600, text), 20sp, followed by an `expand_more` chevron. The whole title is the tap target that opens the switcher. Under it, mono 11.5 dim `main · github.com` (branch · host).
  - Right: `more_vert` (see 3a).
  - No refresh button. Use pull to refresh and the menu item instead.
- **Summary strip:** padding 0 20 12. `ci:` and `deploy:` with their glyphs, an 18dp gap between them, and the age of the view on the right ("8s ago", from `generated_unix_seconds`, ticking).
- **Sheet:** `bg`, top radius 20. The feed:
  - **Section header:** padding top 24 (12 when it follows a commit row), label left inset 26dp (aligned with author names), 12.5/700, letter-spacing +0.2, in the band colour (HEAD `text`, CI Passed `yellow`, Deployed `blue`). `summary` sits on the right, 11 italic dim. Below it a 1dp `line` rule spanning the 20dp margins, then 10dp before the first row.
  - **Commit row:** padding 0 20 14.
    - Line 1 (16dp tall): glyph in a 16dp column, author 12.5/500 dim with 10dp start padding, lead time mono 12 on the right (dim while live, blue once frozen, as today).
    - Line 2: subject 15/20 `text`, 26dp start inset, max 2 lines with ellipsis, 2dp top gap.
  - **Batch label:** left-aligned to the **glyph column** (20dp, the same edge as ✓/✗), matching the TUI. 12.5sp, padding 6–8 top and 6 bottom. Live: bold blue. Settled: italic blue. In flight: italic dim. Failed: italic red. The time is inline ("live on production · deployed 5m ago").
  - **Week divider:** like a section rule but with only a right-aligned label (11 italic dim) above a 1dp `line` rule. 12dp above it, the same as a section header after a row.
  - **Truncation note:** at the end, as today.
- **Status colour rules:** unchanged from `Status.kt`. Header badges are green, red or dim. Row glyphs are always dim except for a non-stale failure, which is red.

### 2h: Repository, several deploy flows
- The title turns red when ci or any flow's deploy is failing (from `broken()`).
- **Strip:** `ci: ✓`, then `deploy:`, then tabs, each with name and glyph (padding 10 12 12, 6dp gap).
  - The selected tab is cut out of the chrome: `bg` fill, top radius 12, flush with the sheet below. The sheet's top-left radius is 0 so the tab joins it seamlessly.
  - Unselected tabs are dim text on the chrome.
  - The strip scrolls horizontally. Swiping the feed horizontally changes flow, and selection is tracked by flow name as today.
  - Undeclared flows show the name followed by "?".

### 2i / 3d: Repository switcher (`ModalBottomSheet`)
- **Sheet:** `surface`, radius 28, with a drag handle.
- **Header:** "Repositories" (20/600) and a tonal "Add" button with the `add` icon.
- **Rows:** padding 12, radius 16, 2dp apart. The current repo has a `line` fill.
  - Left, two lines: `namespace/` (dim) then `name` (600, red if broken), 16sp. Under it, mono 11.5 dim `branch · host`. For a renamed repo it's `namespace/name · branch · host`.
  - Right, 12sp dim: `ci` with its glyph, then `deploy` with its glyphs.
- **Several flows:** show **one glyph per flow, in strip order, with no names** (3d's `acme/monorepo` row). Never hide a flow. The name truncates before any glyph does. The accessibility description lists each flow, e.g. "web passed, ios failed, android none".
- **Long-press a row:** Remove (as today).
- **Below a divider:** a "Device key" row with the `key` icon and the fingerprint on the right. It opens the key screen.
- **Core change needed:** `RepoSummary` has to carry `ci`, `deploy` and per-flow statuses, from the last cached view. Without that, show name and branch only.

### 3a: Overflow menu on the repo screen
An M3 `DropdownMenu`, 252dp wide, radius 12, items 48dp with a leading icon:
1. Refresh now, with "8s ago" on the right
2. Rename…
3. Change branch…
4. Copy clone address
5. A divider, then Remove repository (red icon and text)

### 3b: Rename
- A `ModalBottomSheet` with `imePadding`.
- Title "Rename". Field "Display name", sentence-case keyboard with suggestions, IME action Done.
- Helper text: "Only on this phone. Leave it empty to go back to `acme/web-platform`."
- Actions: Cancel (text) and Save (filled).
- The name is stored locally only. Saving an empty field clears the alias.

### 3c: Remove
- An `AlertDialog` with the `delete` icon (red). Title "Remove web-platform?"
- Body: "Clarity stops watching it and deletes its local copy from this phone. Nothing changes on the host, and the device key stays authorised until you remove it there."
- Actions: Cancel, and Remove as a red text button.

### 3e: Naming rule (pure git, no host detection)
1. Take the path after the host: after `:` in `user@host:path`, or after `host[:port]/` in `ssh://`.
2. Strip a trailing `.git` and `/`, then split on `/`.
3. The last segment is the **name**. Everything before it is the **namespace**, shown dimmed as `namespace/` in front of the name. When space runs out the namespace truncates first; the name never does.
4. No namespace means just the name. The host is always in the subtitle.
5. A rename replaces the title. The derived `namespace/name` moves to the subtitle.

Examples:
- `git@github.com:acme/web-platform.git` → `acme/` **web-platform**
- `ssh://git@gitlab.com/acme/mobile/apps.git` → `acme/mobile/` **apps**
- `git@box.lan:/srv/git/infra.git` → `srv/git/` **infra**
- `pi@nas.local:dotfiles` → **dotfiles**

Ideally the core derives `namespace` and `name` and puts both on `RepoSummary`, so iOS and Android agree.

## Open decisions
- **Status glyphs: text characters (rows 2–3) or icons (row 4).** The icons are Material Symbols *Sharp* at weight 500, optical size 20, with no fill: `check`, `close`, `more_horiz`, and a 4dp square for nothing reported. Sizes are 18dp in commit rows (inside the 16dp column) and 16dp in badges, tabs and the switcher. Colour rules are the same either way. If you pick icons, consider exporting these four as vector drawables rather than shipping the icon font.
- **Fonts:** bundling Roboto Flex and JetBrains Mono, versus the system Roboto and the platform monospace.

## State and data
- `AppState` loses the pager page. It gains: switcher open/closed, connect-flow state (`idle | connecting(step) | hostKeyPrompt(fingerprint) | denied(stderr) | error(msg)`), whether the key has ever connected (drives the key card's default), and the alias per repo.
- **Core and proto additions:**
  - On `RepoSummary`: `namespace`, `display_name`/alias, and cached statuses.
  - A host-key prompt callback during add.
  - A distinct error kind for authentication failure, so the UI can show 2e instead of a generic error.
- Keep today's behaviour: stale views are kept across failed refreshes, every timer ticks from `nowSeconds`, and fetching pauses while the app is in the background.

## Assets
- `assets/mark.png`: copied from `res/mipmap-xxxhdpi/splash_mark.png`. On device, use the existing mipmap or vector.
- `assets/ic_launcher.png`: the existing launcher icon.
- Icons: Google Material Symbols (Rounded for the chrome, Sharp for status glyphs if row 4 is chosen).

## Files
- `Clarity Android.dc.html`: all screens. Open it in a browser and pan or zoom; ids such as 2g are shown as badges.
- `support.js`: the runtime the HTML needs in order to open.
- `assets/`: the images used above.
