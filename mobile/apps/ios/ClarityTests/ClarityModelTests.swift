import XCTest
@testable import Clarity

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

    func testAddingARepositorySelectsItAndClosesTheForm() async {
        let model = model()
        await model.start()
        model.showAddRepo()

        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "")

        XCTAssertNil(model.state.overlay)
        XCTAssertEqual(model.state.repos.map(\.name), ["clarity"])
        XCTAssertEqual(model.state.repos.first?.branch, "main")
        XCTAssertEqual(model.state.selected, model.state.repos.first?.id)
        XCTAssertNil(model.state.error)
    }

    func testARejectedURLKeepsTheFormOpenWithTheReason() async {
        bridge.failAddRepo = #""nonsense" does not look like a git remote — paste the URL you would clone"#

        let model = model()
        await model.start()
        model.showAddRepo()
        await model.addRepo(url: "nonsense", branch: "")

        // Closing it would throw away what was typed, and a paste of a clone
        // URL is not something anyone wants to redo.
        XCTAssertEqual(model.state.overlay, .addRepo)
        XCTAssertTrue(model.state.error?.contains("does not look like a git remote") ?? false)
        XCTAssertTrue(model.state.repos.isEmpty)
    }

    func testOpeningARepositoryReadsDiskBeforeItFetches() async {
        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        await model.addRepo(url: "git@github.com:ezcdlabs/other.git", branch: "main")
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
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.selected!
        bridge.views[id] = FakeBridge.view(of: "yesterday")
        await model.refresh()

        bridge.failSync = "dial tcp: network is unreachable"
        await model.refresh()

        XCTAssertEqual(subject(of: model.state), "yesterday")
        XCTAssertTrue(model.state.error?.contains("network is unreachable") ?? false)
        XCTAssertFalse(model.state.syncing)
    }

    func testAFirstOpenWithNothingOnDiskReportsOnlyTheFetchFailure() async {
        bridge.failSync = "dial tcp: network is unreachable"

        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")

        // Reading a never-fetched repo fails too, but "reference not found" is
        // not news on a first open — it is the expected state, and surfacing it
        // would bury the reason the fetch did not fix it.
        XCTAssertEqual(model.state.error, "dial tcp: network is unreachable")
        XCTAssertNil(model.state.view)
    }

    func testAFetchThatBringsNewCommitsReplacesTheView() async {
        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
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
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        await model.addRepo(url: "git@github.com:ezcdlabs/other.git", branch: "main")
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
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.selected!
        bridge.views[id] = FakeBridge.view(of: "still here")
        await model.refresh()

        model.showAddRepo()
        model.closeOverlay()

        XCTAssertEqual(model.state.selected, id)
        XCTAssertEqual(subject(of: model.state), "still here")
    }

    func testRemovingTheOpenRepositoryFallsBackToAnother() async {
        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        await model.addRepo(url: "git@github.com:ezcdlabs/other.git", branch: "main")
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

    func testTheNextThingThatWorksClearsTheError() async {
        bridge.failAddRepo = "nope"
        let model = model()
        await model.start()
        model.showAddRepo()
        await model.addRepo(url: "nonsense", branch: "")
        XCTAssertEqual(model.state.error, "nope")

        bridge.failAddRepo = nil
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")

        XCTAssertNil(model.state.error)
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
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        bridge.calls.removeAll()

        for _ in 0..<29 { await model.beat() }
        XCTAssertFalse(bridge.calls.contains("sync"), "fetched every beat, not every interval")

        await model.beat()
        XCTAssertTrue(bridge.calls.contains("sync"), "no unprompted fetch: \(bridge.calls)")
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
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.selected!
        bridge.views[id] = FakeBridge.view(of: "yesterday")
        await model.refresh()

        bridge.failSync = "dial tcp: network is unreachable"
        for _ in 0..<30 { await model.beat() }

        // Otherwise the error reappears every thirty seconds for as long as you
        // are on a train, over a dashboard that reads perfectly well.
        XCTAssertNil(model.state.error)
        XCTAssertEqual(subject(of: model.state), "yesterday")
    }

    func testAnAutomaticRefreshThatFailsIsReportedWhenThereIsNothingToRead() async {
        bridge.failSync = "dial tcp: network is unreachable"
        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        model.dismissError()

        for _ in 0..<30 { await model.beat() }

        // With no view behind it, silence would leave an empty screen and no
        // reason for it.
        XCTAssertEqual(model.state.error, "dial tcp: network is unreachable")
    }

    private func subject(of state: AppState) -> String? {
        state.view?.flows.first?.sections.first?.commits.first?.subject
    }
}
