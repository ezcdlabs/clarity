# Mobile: engineering notes

## Where the proto boundary sits

Between the core and the UI, not between the app and the network.

`proto/clarity/v1/view.proto` carries a presentation model: groups already
formed, statuses already resolved, lead times already computed and preformatted.
The rule is **decisions in Go, geometry native**. Anything that decides what is
true crosses the boundary settled; anything that decides how wide, what colour or
which glyph is the platform's business.

That split is not tidiness. Most of the terminal renderer's logic exists because
a terminal quantises to character cells — reserving a column for a clamp arrow,
guaranteeing a one-cell minimum bar, shedding tick labels that would collide.
None of it has an analogue at pixel resolution, and none of it should cross.

What crosses is the terminal's own structure, not a flattening of it. Three
lifecycle bands — HEAD, CI Passed, Deployed — always all three and always in
that order, because they are the frame the view is read against: an empty
Deployed says "nothing has shipped", which is an answer, and a client handed
only the non-empty bands could not tell that from a band that went missing.
Inside them, each deploy is its own batch with the subheader the TUI writes
above it, and exactly one batch is marked live — what is running in production
now, as against what merely happened.

Getting that wrong is what the first mobile build shipped: five deploys became
five sections all labelled "Deployed", so the phone repeated the word down the
screen while the terminal says it once.

A commit carries its CI status and no deploy status, which is not an omission.
The terminal renderer has never drawn one: *"the deploy status is implied by the
section the row sits in"*. Two marks per row say the same thing twice, so the
field is absent rather than ignored.

Protobuf also sidesteps gomobile's type system. Only strings, bytes, ints, bools,
errors and struct pointers cross an FFI boundary it generates, so a rich view
would otherwise need a hand-written wrapper per platform — three places for the
same shape to drift. Encoded bytes need none, and one schema generates all three
sides.

## Structure

```
mobile/
├── core/                 # the FFI surface gomobile binds. No logic.
├── internal/
│   ├── gitsource/        # go-git source: fetch, then read what was fetched
│   ├── present/          # core.View -> clarity.v1.View
│   ├── keys/             # the device's ed25519 identity
│   └── registry/         # which repositories this device tracks
└── apps/
    ├── android/          # Gradle project, Kotlin + Compose
    └── ios/              # Xcode project, Swift + SwiftUI
```

`mobile/core` is deliberately empty of decisions — every method translates an
FFI-friendly shape into a call on `mobile/internal/*`, which is where behaviour
lives and where it can be tested without a device. Generated protobuf is
committed, in Go and in each app's own language, the same way `proto/gen/go` is.

So are the app icons, from the ezcd mark in `mobile/internal/icon` — written out
at every size both platforms want by `go run ./mobile/tools/gen-icons`.

## The mark is geometry, not a file

Three identical checks offset along one axis on a navy disc: white at the back,
then yellow, then blue in front. The two at the back are clipped to the disc —
the white one runs off its lower left and is cut there — while the blue one is
allowed past the edge at the top right. That asymmetry is the composition rather
than an accident of drawing: the mark reads as layers, and layers need something
to be in front of.

The constants came off the original artwork by fitting the *edges* of each arm,
not by finding its corners. Antialiasing erodes a sharp corner by a pixel or
two, so corners read as consistently inside where the geometry really is, while
a long straight edge fits exactly. Both arms turn out to sit at precisely 45°,
the ends are cut square, the elbow is a mitre, and the checks are 33px apart on
a 180px mark.

The one thing that could not have been got by eye is the **moat**: every check
carries a navy clearance that cuts the one behind it short. Two parallel arms
are already far enough apart to show navy between them, but where a back check's
long arm passes behind a front check's short arm they would meet and merge into
a single shape. Modelling the moat took the reconstruction from 3.4% of pixels
wrong to 1.6% — the difference between three checks that happen to be near each
other and a stack that reads as layers.

Holding it as geometry rather than as a PNG is what lets one definition serve an
Android adaptive foreground, two legacy densities, an iOS app icon and a splash
screen, and makes a change to it a readable diff.

## Each format wants the mark differently

Three grounds, because the formats disagree about what may be behind the mark.
Transparent outside the disc is the mark as the brand uses it. Navy to the edges
is for anywhere transparency is refused — an iOS app icon is rejected outright
if any pixel is see-through — and the navy reaching the corners is the same navy
the disc is made of. Ink only, with every navy pixel including the moats left
transparent, is what an adaptive icon's foreground is: Android paints the
background layer and moves it independently, so a foreground carrying its own
disc would be a second navy that cannot follow the first.

The adaptive foreground is the one place the blue check is clipped like the
others, and the one place sizing is a judgement rather than a rule. Android
guarantees only the inner 66dp of a 108dp canvas, but it shows 72dp — and the
strict safe zone is for a mark with a subject floating in space, which this is
not. Sized to it, the mark sat adrift in a ring of empty navy and was barely
legible at 48dp, which is the size that decides whether an icon works. So the
disc is mapped onto the mask instead: the mask plays the part the disc plays in
the mark, a tighter launcher crops navy the way a tighter mask crops any
circular logo, and the blue overflow goes — because a check cannot be seen
escaping the very thing that is cutting it.

A launch screen is the mark on the theme's own ground. It keeps its disc, unlike
the adaptive foreground: the ground follows light and dark, and on a light one
an ink-only mark would hand the white check a white background to disappear
into. It is also sized smaller than the adaptive foreground, and for the
opposite reason — an adaptive icon is definitely masked, so the mark is fitted
to the mask; a launch screen may or may not be, so nothing may rely on it and
the whole mark including the blue overflow sits far enough in that a mask would
have nothing to cut.

Android 12 took splash screens over and ignores a window background set for the
purpose, so there are two: a layer-list for the launch theme on older versions,
and the v31 attributes above it. Only the launch style is duplicated into `values-v31`, not
the app's theme — night mode outranks platform version when Android picks a
resource folder, so a theme copied there would lose its dark variant on every
modern device.

The bound archive is not committed. `scripts/bind-android.sh` produces a 33 MB
`.aar` of per-ABI native code; checking it in would make every bind a binary
diff.

## A bridge interface, so the UI is testable without a device

Each app talks to an interface — `ClarityBridge`, in Kotlin and in Swift — not
to the generated bindings. Behind it sits either the real bound core or a fake, and the
screens cannot tell which.

This is what makes the interesting behaviour testable on a laptop. The state
machine in front of the bridge decides things that only show up on a bad day: a
rejected URL keeps you on the add screen so a pasted clone URL is not lost; a
failed refresh keeps the view it already had, because on a phone yesterday's
pipeline beats an empty screen with an error on it; a first open of a
never-fetched repository reports why the *fetch* failed, not the "reference not
found" that reading an empty store always produces.

The fake makes every one of those a unit test rather than something you find out
about on a train.

What the fake deliberately does *not* do is re-decide anything the core already
decided. The registry validates URLs and writes the message explaining a
rejection; the app surfaces that message verbatim. A second opinion in Kotlin
could only disagree with it.

## Two apps, one set of decisions

Android and iOS draw the same screens from the same view, and how alike they
should be answers itself once you ask what two teams working from one brief
would have in common anyway: the proto, the model's shape, every status and
grouping decision, the palette roles, the feed's grid and its vertical rhythm.
Those are shared. The state machine in front of the bridge is a near
line-for-line port with the same test suite behind it, because the two make the
same decisions and asking both the same questions is the cheapest way to find
out when one of them has stopped.

What is deliberately *not* shared is any component. Each app uses what its
platform has already taught the reader to read:

| the decision | Android | iOS |
| --- | --- | --- |
| the repository switcher | `ModalBottomSheet`, remove behind a long press | `List` in a sheet, remove behind a swipe |
| refreshing | `PullToRefreshBox` | `.refreshable` |
| the per-repository menu | `DropdownMenu` with a trailing note per row | `Menu`, the note folded into the label |
| the host-key prompt | `AlertDialog` | a sheet |
| the deploy targets | tabs cut out of the chrome, flush with the feed | capsule chips |
| feed against chrome | a rounded sheet that slides under the bar | two tones |
| type and marks | Roboto Flex, JetBrains Mono, Material Symbols | SF Pro, SF Mono, SF Symbols |

Two of those are worth saying why. The host-key prompt is a sheet on iOS
because a UIKit alert takes a title, a message and buttons, and this one has a
fingerprint block that has to be read character by character — an alert would
flatten it into prose exactly where it must not be. And the deploy targets are
chips rather than the obvious segmented `Picker` because a segmented control
takes a label *or* an image, never both: the per-flow result would have had to
leave the control, and then nothing would say which flow the failing mark
belonged to.

The one thing the platform forced rather than suggested: iOS presents all three
overlays through a single `sheet(item:)`. Three sheets with a boolean each
cannot swap — going from the switcher straight to the connect flow dismisses one
and presents another in the same frame, SwiftUI reconciles one presentation per
turn, and the second is simply lost.

## Telling you when it breaks

The Android app checks in the background and notifies when a pipeline crosses
between green and red.

**Transitions, not state.** A check runs on a timer whether or not anything
happened, so a notification keyed on state is one that fires four times an hour
for as long as a build stays broken — which is a notification the user turns
off, taking the useful ones with it. `mobile/internal/watch` compares the
verdict the registry already caches for the switcher against the fresh one and
reports only the crossings.

Only `passed` ↔ `failed` counts. A build in flight is not an answer, and
treating the gap between a push and a green tick as a recovery would fire on
every commit. It also gives the common shape of a broken pipeline the right
behaviour for free: red, someone pushes a fix, it runs, it fails again — the
second failure arrives from `started` rather than from `passed`, so it is
announced once. A repository with no recorded verdict is silent too: it did not
break, it was always like that.

**Fifteen minutes, and that is a floor rather than a choice.** WorkManager will
not schedule periodic work more often, and Doze routinely stretches it further
when the phone is idle. The alternative is a foreground service holding a
permanent notification, which is a steep price for a dashboard. There is no push
channel to fall back on, and there should not be: that would need a server, and
clarity does not have one.

**Two channels, and no alarm.** Breaking is `IMPORTANCE_HIGH` — a heads-up with
a sound, because it is the thing the feature exists for. Recovering is
`IMPORTANCE_DEFAULT`, silent, because good news can wait until you look.
Splitting them puts the dial where Android wants it: the user mutes one and
keeps the other from system settings, and the app needs no preferences screen.

**The repository on screen gets a chime, not a notification.** The rule is the
one every messaging app uses: a short tone for the conversation you are in, a
notification for the ones you are not. A notification is for something you would
otherwise miss, and a feed that turned red in front of you is not that — what
you might miss is the *moment*, with the phone face-up on a desk while you look
at a different screen. So it is a brief tone on the notification stream (silent
mode and Do Not Disturb still apply) plus a haptic: two taps for broken, one for
recovered. Deliberately not the notification tone, because hearing that and
finding nothing in the shade is worse than hearing nothing.

That needs `Read` rather than `View`, and the reason is a trap worth naming.
Reading a view is what advances the baseline — the verdict is recorded so the
switcher can show it — so a pipeline that breaks while its own screen is open
has its crossing consumed by the very read that drew it red, and the background
check afterwards compares against a baseline that has already moved. Before
`Read`, the repository you were watching was the one repository that could never
announce anything, by accident rather than by design. `Read` hands the crossings
back alongside the view, and only a *background* refresh announces them: a chime
the instant a screen opens would be reporting history as news, and the first
read after an install would play the whole backlog at once.

What it deliberately does not do is ring like an alarm or bring itself to the
foreground. Android has a mechanism for that — a full-screen intent — and since
14 the permission for it is granted by default only to calling and alarm apps.
That restriction is right. A check that is up to forty minutes behind cannot
honestly claim the urgency an alarm claims, and waking someone at three in the
morning over something that broke twenty minutes ago is the behaviour of an
alarm clock rather than of a dashboard.

The status bar icon is generated from the mark like every other icon, but as the
check alone: a small icon is drawn as a white mask, and at 24dp three
overlapping checks are mud once the colours that tell them apart are gone.

iOS does not have this yet. Its equivalent is `BGAppRefreshTask`, which is
scheduled at the system's discretion rather than on an interval.

## Authentication: a key per device

The app generates an ed25519 keypair on first use and shows the
`authorized_keys` line to paste into whichever host the user uses. There is no
provider integration, no OAuth app and no server, because **clarity is not a
GitHub tool** — a key the user installs themselves works on GitHub, GitLab and a
box in a cupboard equally. The private half never leaves the device, and an
https remote gets no credential at all.

The host key is checked against a `known_hosts` the app keeps itself. go-git's
default is to read `~/.ssh/known_hosts`, which on a phone does not exist, so
every fetch fails before it reaches the network: *unable to find any valid
known_hosts file*. The obvious shortcut is `ssh.InsecureIgnoreHostKey`, and it
is the wrong one — a tool whose job is telling you the truth about your pipeline
should not accept whatever answers on port 22.

So the app does what an ssh client does on first contact: trust the key it is
shown, write it down, and refuse anything different afterwards. That does not
protect the very first fetch, which nothing short of a key the user types in
ever could. It does mean a host whose key changes under you is reported rather
than silently accepted, which is the case that actually happens.

## Nothing that crosses the boundary may be empty

gomobile cannot carry a zero-length byte slice. Its `fromSlice` turns one into a
null `jbyteArray` on Android and a nil `NSData` on iOS, and a protobuf message
with no fields set encodes to exactly zero bytes — so a perfectly legal payload
arrives as null and the decoder throws. The first thing a new install asks for
is the repository list, which is empty, so this is the common path rather than
an edge of it.

Both `View` and `RepoList` therefore carry `generated_unix_seconds`, always set.
It is not only ballast: a mobile client holds the last good view across a failed
refresh, so how old the thing on screen is happens to be exactly what the user
needs to know at that moment.

## iOS without a Mac in the loop

Development happens on Linux, where there is no `swift`, no `xcodebuild` and no
iOS SDK, so iOS source is written blind. The correctness gate is a macOS CI
runner, added with the first iOS commit rather than after it — otherwise the
first person to open the project on a Mac inherits every mistake at once.

Two consequences follow from that, and both are deliberate:

**The `.xcodeproj` is generated, not committed.** `project.yml` is the source of
truth and XcodeGen produces the project from it. A `pbxproj` is a UUID soup that
cannot be reviewed and cannot be opened at all on the machine it would be edited
from.

**The Swift protobuf is generated in the build, not committed.** This is the one
place the "commit the generated output" rule does not hold, and the reason is
the same one: `protoc-gen-swift` is itself a Swift binary, so it cannot run
where the output would be committed from. Committing output nobody present can
regenerate is worse than regenerating it, so `scripts/gen-ios-proto.sh` runs on
the macOS runner before every iOS build — which also means the app is always
compiled against the current schema rather than a stale copy of it.
