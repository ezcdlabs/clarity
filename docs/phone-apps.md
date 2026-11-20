# The phone apps

clarity's Android and iOS apps show the same commit-centric pipeline view as
the terminal, fetched over SSH straight from your repository. They ship from
this repository's own [GitHub releases](https://github.com/ezcdlabs/clarity/releases)
— there is no app store listing.

## Android

Download `clarity-android.apk` from the latest release and open it; Android
asks once to allow installs from your browser. From then on **the app updates
itself**: when a newer release exists, the repository switcher (tap the repo
name) shows an "Update to x.y.z" row, and installing it goes through
Android's own confirmation dialog. The system verifies every update is signed
with the same key as the installed app.

If you prefer a store-shaped updater, [Obtainium](https://github.com/ImranR98/Obtainium)
pointed at `ezcdlabs/clarity` reads the same releases.

## iOS

The iOS app installs through [AltStore](https://altstore.io) (Classic), which
sideloads apps by re-signing them with your own Apple ID:

1. Install AltStore on your phone via [AltServer](https://altstore.io) on a
   computer.
2. In AltStore, add this source:

       https://ezcdlabs.github.io/clarity/apps.json

3. Install clarity from the source. Updates appear in AltStore whenever a
   release is cut.

What sideloading with a free Apple ID means in practice: the app expires
every 7 days and AltStore refreshes it (automatically, while AltServer is
reachable on your network); a free account can hold 3 sideloaded apps; and
iOS withholds push notifications from sideloaded apps, so the background
"pipeline went red" notifications the Android app gets are not available on
iOS. [SideStore](https://sidestore.io) works with the same source URL.
