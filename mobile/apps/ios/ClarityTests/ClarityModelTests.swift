import XCTest
@testable import Clarity

/// The same suite the Android model has, against the same fake.
///
/// Deliberately a port rather than a different set of tests: the two models
/// make the same decisions, and the cheapest way to find out that one of them
/// has stopped is to ask both the same questions.
@MainActor
final class ClarityModelTests: XCTestCase {

    private var bridge = FakeBridge()
    private var now: Int64 = 1_700_000_000

    override func setUp() {
        super.setUp()
        bridge = FakeBridge()
        now = 1_700_000_000
    }

    private func model() -> ClarityModel {
        ClarityModel(bridge: bridge, clock: { self.now })
    }

    func testLaunchingLandsInARepositoryRatherThanOnAMenu() async throws {
        _ = try bridge.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        _ = try bridge.addRepo(url: "git@github.com:ezcdlabs/other.git", branch: "main")

        let model = model()
        await model.start()

        XCTAssertEqual(model.state.repos.map(\.name), ["clarity", "other"])
        // There is nothing to read on a menu, and one commit list is the thing
        // the app exists to show.
        XCTAssertEqual(model.state.selected, model.state.repos.first?.id)
    }

    func testConnectingARepositorySelectsItAndClosesTheFlow() async {
        let model = model()
        await model.start()
        model.showConnect()

        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "")

        XCTAssertNil(model.state.overlay)
        XCTAssertEqual(model.state.repos.map(\.name), ["clarity"])
        XCTAssertEqual(model.state.repos.first?.branch, "main")
        XCTAssertEqual(model.state.selected, model.state.repos.first?.id)
        XCTAssertNil(model.state.error)
    }

    func testARejectedURLKeepsTheFlowOpenWithTheReason() async {
        bridge.failAddRepo = #""nonsense" does not look like a git remote — paste the URL you would clone"#

        let model = model()
        await model.start()
        model.showConnect()
        await model.connect(url: "nonsense", branch: "")

        // Closing it would throw away what was typed, and a paste of a clone
        // URL is not something anyone wants to redo. The reason belongs to the
        // flow rather than to the error bar, which is behind it.
        XCTAssertEqual(model.state.overlay, .connect)
        guard case let .failed(message) = model.state.connect else {
            return XCTFail("connect = \(model.state.connect)")
        }
        XCTAssertTrue(message.contains("does not look like a git remote"))
        XCTAssertTrue(model.state.repos.isEmpty)
    }

    func testOpeningARepositoryReadsDiskBeforeItFetches() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        await model.connect(url: "git@github.com:ezcdlabs/other.git", branch: "main")
        let first = model.state.repos[0].id
        bridge.views[first] = FakeBridge.view(of: "what we already had")
        bridge.calls.removeAll()

        await model.select(first)

        // The point of a local object store: offline, the last fetch is still
        // the dashboard. A spinner over an empty screen would waste it.
        XCTAssertEqual(bridge.calls.first, "view")
        XCTAssertEqual(bridge.calls.dropFirst().first, "sync")
    }

    func testAFailedRefreshKeepsTheLastViewAndSaysWhatHappened() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.selected!
        bridge.views[id] = FakeBridge.view(of: "yesterday")
        await model.refresh()

        bridge.failSync = "dial tcp: network is unreachable"
        await model.refresh()

        XCTAssertEqual(subject(of: model.state), "yesterday")
        XCTAssertTrue(model.state.error?.contains("network is unreachable") ?? false)
        XCTAssertFalse(model.state.syncing)
    }

    func testAFirstConnectionWithNothingOnDiskReportsOnlyTheFetchFailure() async {
        bridge.failSync = "dial tcp: network is unreachable"

        let model = model()
        await model.start()
        model.showConnect()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")

        // The flow stays open on the reason. Reading a never-fetched repo fails
        // too, but "reference not found" is not news on a first connection —
        // it is the expected state, and surfacing it would bury the reason the
        // fetch did not fix it.
        XCTAssertEqual(model.state.overlay, .connect)
        XCTAssertEqual(model.state.connect, .failed(message: "dial tcp: network is unreachable"))
        XCTAssertNil(model.state.view)
    }

    func testAFetchThatBringsNewCommitsReplacesTheView() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.selected!
        bridge.views[id] = FakeBridge.view(of: "yesterday")
        await model.refresh()

        bridge.onSync = { [bridge] repoID in bridge.views[repoID] = FakeBridge.view(of: "today") }
        await model.refresh()

        XCTAssertEqual(subject(of: model.state), "today")
    }

    func testSwitchingRepositoriesDropsThePreviousOnesCommits() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        await model.connect(url: "git@github.com:ezcdlabs/other.git", branch: "main")
        let first = model.state.repos[0].id
        let second = model.state.repos[1].id
        bridge.views[first] = FakeBridge.view(of: "clarity's work")
        await model.select(first)
        XCTAssertEqual(subject(of: model.state), "clarity's work")

        await model.select(second)

        // Showing one repository's commits under another's name is worse than
        // showing nothing at all.
        XCTAssertEqual(model.state.selected, second)
        XCTAssertNil(model.state.view)
    }

    func testTheSelectionSurvivesAnOverlay() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.selected!
        bridge.views[id] = FakeBridge.view(of: "still here")
        await model.refresh()

        model.showSwitcher()
        model.closeOverlay()

        XCTAssertEqual(model.state.selected, id)
        XCTAssertEqual(subject(of: model.state), "still here")
    }

    func testRemovingTheOpenRepositoryFallsBackToAnother() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        await model.connect(url: "git@github.com:ezcdlabs/other.git", branch: "main")
        let open = model.state.selected!

        await model.removeRepo(open)

        XCTAssertEqual(model.state.repos.map(\.name), ["clarity"])
        XCTAssertNotEqual(model.state.selected, open)
        XCTAssertEqual(model.state.selected, model.state.repos.first?.id)
    }

    func testTheKeyScreenGeneratesAKeyOnDemand() async {
        let model = model()
        await model.start()
        XCTAssertFalse(bridge.keyExists)

        await model.showKey()

        XCTAssertEqual(model.state.overlay, .key)
        XCTAssertTrue(model.state.publicKey?.hasPrefix("ssh-ed25519 ") ?? false)
        XCTAssertTrue(bridge.keyExists)
    }

    func testTheNextAttemptThatWorksClearsTheReasonTheLastOneFailed() async {
        bridge.failAddRepo = "nope"
        let model = model()
        await model.start()
        model.showConnect()
        await model.connect(url: "nonsense", branch: "")
        XCTAssertEqual(model.state.connect, .failed(message: "nope"))

        bridge.failAddRepo = nil
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")

        // Connected: the flow closes, and nothing is left saying it did not.
        XCTAssertEqual(model.state.connect, .idle)
        XCTAssertNil(model.state.overlay)
        XCTAssertNil(model.state.error)
    }

    func testAnUnknownHostStopsTheFlowAndOffersTheFingerprint() async {
        bridge.syncOutcome = .hostKeyUnknown

        let model = model()
        await model.start()
        model.showConnect()
        await model.connect(url: "git@git.acme.dev:acme/thing.git", branch: "main")

        // Nothing is trusted yet — the whole point of asking is that it can be
        // answered no.
        guard case let .askHost(key, changed) = model.state.connect else {
            return XCTFail("connect = \(model.state.connect)")
        }
        XCTAssertEqual(key.host, "git.acme.dev")
        XCTAssertTrue(key.fingerprint.hasPrefix("SHA256:"))
        XCTAssertFalse(changed)
        XCTAssertFalse(bridge.calls.contains("trustHost"), "trusted without being asked")

        bridge.syncOutcome = .ok
        await model.trustHost()

        XCTAssertTrue(bridge.calls.contains("trustHost"))
        XCTAssertEqual(model.state.connect, .idle)
        XCTAssertNil(model.state.overlay)
        XCTAssertEqual(model.state.selected, model.state.repos.first?.id)
    }

    func testARefusedKeyLandsOnTheScreenThatExplainsIt() async {
        bridge.syncOutcome = .authDenied
        bridge.gitOutput = "Permission denied (publickey)."

        let model = model()
        await model.start()
        model.showConnect()
        await model.connect(url: "git@github.com:acme/thing.git", branch: "main")

        // The raw words, so whoever is debugging a key sees what the host said.
        XCTAssertEqual(model.state.connect, .denied(gitOutput: "Permission denied (publickey)."))
        XCTAssertEqual(model.state.overlay, .connect)

        // Retrying after fixing it on the host picks up where it stopped.
        bridge.syncOutcome = .ok
        await model.retryConnect()
        XCTAssertNil(model.state.overlay)
    }

    func testConnectingOnceIsWhatFoldsTheKeyAwayNextTime() async {
        let model = model()
        await model.start()
        XCTAssertFalse(model.state.keyHasConnected, "the key should start on show")

        model.showConnect()
        await model.connect(url: "git@github.com:acme/thing.git", branch: "main")

        // After a host has accepted it, the key is a fact you occasionally
        // check rather than the thing you are here to copy.
        XCTAssertTrue(model.state.keyHasConnected)
    }

    func testTrustingAHostRetriesTheRepositoryBeingConnected() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:acme/zebra.git", branch: "main")

        bridge.syncOutcome = .hostKeyUnknown
        model.showConnect()
        await model.connect(url: "git@git.acme.dev:acme/aardvark.git", branch: "main")
        let aardvark = model.state.repos.first { $0.name == "aardvark" }!.id

        bridge.syncOutcome = .ok
        var synced: [String] = []
        bridge.onSync = { synced.append($0) }
        await model.trustHost()

        // The list is ordered by the name a row shows, so the repository that
        // was just added is not the one at the end of it. Picking up the wrong
        // one here would trust a host on behalf of one repository and then go
        // and fetch a different one.
        //
        // Deduplicated: a connection that succeeds fetches once to find out and
        // once to open, which is two calls about one repository rather than one
        // call about two.
        XCTAssertEqual(Set(synced), [aardvark])
        XCTAssertEqual(model.state.selected, aardvark)
    }

    func testRetryingAfterARefusedKeyRetriesTheRepositoryBeingConnected() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:acme/zebra.git", branch: "main")

        bridge.syncOutcome = .authDenied
        model.showConnect()
        await model.connect(url: "git@git.acme.dev:acme/aardvark.git", branch: "main")
        let aardvark = model.state.repos.first { $0.name == "aardvark" }!.id

        bridge.syncOutcome = .ok
        var synced: [String] = []
        bridge.onSync = { synced.append($0) }
        await model.retryConnect()

        XCTAssertEqual(Set(synced), [aardvark])
        XCTAssertEqual(model.state.selected, aardvark)
    }

    func testRenamingIsLocalAndClearable() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:acme/web-platform.git", branch: "main")
        let id = model.state.repos[0].id

        await model.rename(id, to: "The Platform")
        XCTAssertEqual(model.state.repos[0].alias, "The Platform")

        await model.rename(id, to: "")
        XCTAssertEqual(model.state.repos[0].alias, "")
    }

    func testChangingBranchThrowsTheOldBranchsCommitsAway() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:acme/thing.git", branch: "main")
        let id = model.state.repos[0].id
        bridge.views[id] = FakeBridge.view(of: "on main")
        await model.refresh()
        XCTAssertEqual(subject(of: model.state), "on main")

        bridge.views[id] = FakeBridge.view(of: "on release")
        await model.changeBranch(id, to: "release")

        XCTAssertEqual(model.state.repos[0].branch, "release")
        // Leaving main's commits under release's name would be the most
        // confusing possible outcome.
        XCTAssertEqual(subject(of: model.state), "on release")
    }

    func testTheSwitcherSeesWhatTheLastViewSaid() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:acme/thing.git", branch: "main")
        let id = model.state.repos[0].id
        bridge.views[id] = FakeBridge.view(of: "something")

        await model.refresh()

        // The verdict is written down when a view is read, so the list that
        // shows it has to be re-read afterwards. Without that the switcher says
        // "nothing reported" about a repository whose own screen is showing a
        // green tick.
        XCTAssertEqual(model.state.repos[0].ci, .passed)
    }

    func testAMenuThatCannotBeReadIsReportedRatherThanShownEmpty() async {
        bridge.failList = "open repos.json: permission denied"

        let model = model()
        await model.start()

        XCTAssertTrue(model.state.error?.contains("permission denied") ?? false)
    }

    // MARK: - the clock and the automatic refresh

    func testTheClockAdvancesOnEveryBeat() async {
        let model = model()
        await model.start()

        await model.beat()
        XCTAssertEqual(model.state.nowSeconds, now)

        now += 5
        await model.beat()

        // Every timer on screen is formatted against this, so they tick
        // together and the whole screen stays consistent with itself.
        XCTAssertEqual(model.state.nowSeconds, now)
    }

    func testTheOpenRepositoryRefetchesOnItsOwn() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        bridge.calls.removeAll()

        // Four beats short of the interval, so the only thing that can make the
        // next assertion pass is the interval itself rather than any beat.
        for _ in 0..<4 { await model.beat() }
        XCTAssertFalse(bridge.calls.contains("sync"), "fetched every beat, not every interval")

        await model.beat()
        XCTAssertTrue(bridge.calls.contains("sync"), "no unprompted fetch: \(bridge.calls)")
    }

    func testComingBackToTheAppRefetchesAtOnce() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        bridge.calls.removeAll()

        await model.catchUp()

        // No beats: the fetch has to be part of coming back rather than the
        // first tick of the interval. A phone spends most of its life in a
        // pocket, so what is on screen when you look at it is as old as the
        // last time you looked.
        XCTAssertTrue(bridge.calls.contains("sync"), "waited for the interval: \(bridge.calls)")
    }

    func testNothingTicksWhileTheAppIsInTheBackground() async {
        let model = model()
        await model.start()
        model.resume()
        XCTAssertTrue(model.isTicking)

        model.pause()

        // A timer nobody can see only spends battery, and a fetch nobody asked
        // for spends their data too.
        XCTAssertFalse(model.isTicking)
    }

    func testAnAutomaticRefreshThatFailsDoesNotNagOverDataYouCanRead() async {
        let model = model()
        await model.start()
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.selected!
        bridge.views[id] = FakeBridge.view(of: "yesterday")
        await model.refresh()

        bridge.failSync = "dial tcp: network is unreachable"
        for _ in 0..<5 { await model.beat() }

        // Otherwise the error bar reappears every few seconds for as long as
        // you are on a train, over a dashboard that reads perfectly well.
        XCTAssertNil(model.state.error)
        XCTAssertEqual(subject(of: model.state), "yesterday")
    }

    func testAnAutomaticRefreshThatFailsIsReportedWhenThereIsNothingToRead() async {
        let model = model()
        await model.start()
        // Connected, but nothing on disk to read — so the screen behind the
        // failure is empty, which is the case that has to speak up.
        await model.connect(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        bridge.failSync = "dial tcp: network is unreachable"
        model.dismissError()

        for _ in 0..<5 { await model.beat() }

        // With no view behind it, silence would leave an empty screen and no
        // reason for it.
        XCTAssertEqual(model.state.error, "dial tcp: network is unreachable")
    }

    private func subject(of state: AppState) -> String? {
        state.view?.flows.first?.sections.first?.commits.first?.subject
    }
}
