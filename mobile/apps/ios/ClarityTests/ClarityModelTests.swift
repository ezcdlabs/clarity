import XCTest
@testable import Clarity

@MainActor
final class ClarityModelTests: XCTestCase {

    private var bridge = FakeBridge()

    override func setUp() {
        super.setUp()
        bridge = FakeBridge()
    }

    private func model() -> ClarityModel { ClarityModel(bridge: bridge) }

    func testTheMenuShowsWhatWasAlreadyTracked() async throws {
        _ = try bridge.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")

        let model = model()
        await model.start()

        XCTAssertEqual(model.state.repos.map(\.name), ["clarity"])
        XCTAssertEqual(model.state.screen, .repos)
    }

    func testAddingARepositoryListsItAndReturnsToTheMenu() async {
        let model = model()
        await model.start()
        model.showAddRepo()

        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "")

        XCTAssertEqual(model.state.screen, .repos)
        XCTAssertEqual(model.state.repos.map(\.name), ["clarity"])
        XCTAssertEqual(model.state.repos.first?.branch, "main")
        XCTAssertNil(model.state.error)
    }

    func testARejectedURLKeepsYouOnTheAddScreenWithTheReason() async {
        bridge.failAddRepo = #""nonsense" does not look like a git remote — paste the URL you would clone"#

        let model = model()
        await model.start()
        model.showAddRepo()
        await model.addRepo(url: "nonsense", branch: "")

        // Staying put matters: navigating away would throw away what was typed,
        // and a paste of a clone URL is not something anyone wants to redo.
        XCTAssertEqual(model.state.screen, .addRepo)
        XCTAssertTrue(model.state.error?.contains("does not look like a git remote") ?? false)
        XCTAssertTrue(model.state.repos.isEmpty)
    }

    func testOpeningARepositoryReadsDiskBeforeItFetches() async {
        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.repos[0].id
        bridge.views[id] = FakeBridge.view(of: "what we already had")
        bridge.calls.removeAll()

        await model.openRepo(id)

        // The point of a local object store: offline, the last fetch is still
        // the dashboard. A spinner over an empty screen would waste it.
        XCTAssertEqual(bridge.calls.first, "view")
        XCTAssertEqual(bridge.calls.dropFirst().first, "sync")
    }

    func testAFailedRefreshKeepsTheLastViewAndSaysWhatHappened() async {
        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.repos[0].id
        bridge.views[id] = FakeBridge.view(of: "yesterday")
        await model.openRepo(id)

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
        let id = model.state.repos[0].id

        await model.openRepo(id)

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
        let id = model.state.repos[0].id
        bridge.views[id] = FakeBridge.view(of: "yesterday")
        await model.openRepo(id)

        bridge.onSync = { [bridge] repoID in bridge.views[repoID] = FakeBridge.view(of: "today") }
        await model.refresh()

        XCTAssertEqual(subject(of: model.state), "today")
    }

    func testLeavingARepositoryDropsItsView() async {
        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        let id = model.state.repos[0].id
        bridge.views[id] = FakeBridge.view(of: "yesterday")
        await model.openRepo(id)

        model.back()

        // Otherwise opening a second, never-fetched repo would briefly show the
        // first one's commits.
        XCTAssertEqual(model.state.screen, .repos)
        XCTAssertNil(model.state.view)
    }

    func testRemovingARepositoryDropsItFromTheMenu() async {
        let model = model()
        await model.start()
        await model.addRepo(url: "git@github.com:ezcdlabs/clarity.git", branch: "main")
        await model.addRepo(url: "git@github.com:ezcdlabs/other.git", branch: "main")
        let id = model.state.repos[0].id

        await model.removeRepo(id)

        XCTAssertEqual(model.state.repos.map(\.name), ["other"])
    }

    func testTheKeyScreenGeneratesAKeyOnDemand() async {
        let model = model()
        await model.start()
        XCTAssertFalse(bridge.keyExists)

        await model.showKey()

        XCTAssertEqual(model.state.screen, .key)
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

    private func subject(of state: AppState) -> String? {
        state.view?.flows.first?.groups.first?.commits.first?.subject
    }
}
