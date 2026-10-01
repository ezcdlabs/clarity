import Foundation

/// The app's state machine.
///
/// Holds no SwiftUI types and no bindings, so the whole of it is testable
/// against `FakeBridge`. The views read `state` and call these methods; they
/// make no decisions of their own.
///
/// Every method is `async` and completes before it returns, which is what makes
/// a test a sequence of statements rather than a wait for an expectation.
@MainActor
final class ClarityModel: ObservableObject {
    @Published private(set) var state = AppState()

    private let bridge: ClarityBridge

    /// How many commits a repository screen asks for.
    private let commitLimit = 200
    /// Zero lets the core pick its own fetch depth and timeout.
    private let fetchDepth = 0
    private let fetchTimeoutSeconds = 0

    init(bridge: ClarityBridge) {
        self.bridge = bridge
    }

    func start() async {
        await act { try self.loadRepos() }
    }

    func showAddRepo() {
        state.screen = .addRepo
        state.error = nil
    }

    func showKey() async {
        state.screen = .key
        state.error = nil
        await act {
            // Asking for the key is what creates it. There is no point
            // generating one for a user who never adds an ssh remote.
            let key = try self.bridge.publicKey(comment: "clarity on ios")
            self.state.publicKey = key
        }
    }

    /// Back to the menu, dropping whatever the last screen was showing.
    func back() {
        state.screen = .repos
        state.view = nil
        state.error = nil
    }

    func addRepo(url: String, branch: String) async {
        await act {
            _ = try self.bridge.addRepo(url: url, branch: branch)
            try self.loadRepos()
            self.state.screen = .repos
        }
    }

    func removeRepo(_ repoID: String) async {
        await act {
            try self.bridge.removeRepo(repoID)
            try self.loadRepos()
            if self.state.screen == .repo(id: repoID) {
                self.state.screen = .repos
                self.state.view = nil
            }
        }
    }

    /// Opens a repository: what is already on disk first, then a fetch.
    ///
    /// The order is the point. A phone is offline often, and the local object
    /// store is what makes that survivable — showing the last fetch immediately
    /// beats a spinner over nothing.
    func openRepo(_ repoID: String) async {
        state.screen = .repo(id: repoID)
        state.view = nil
        state.error = nil

        // A repository that has never been fetched has no branch to resolve, so
        // this fails. That is the expected state on a first open, not something
        // to report: the fetch about to run is the answer to it, and if the
        // fetch fails too, its reason is the useful one.
        if let onDisk = readView(repoID) {
            state.view = onDisk
        }
        await fetch(repoID)
    }

    /// Re-fetches the open repository.
    func refresh() async {
        guard case let .repo(id) = state.screen else { return }
        await fetch(id)
    }

    func dismissError() {
        state.error = nil
    }

    // MARK: - internals

    private func fetch(_ repoID: String) async {
        state.syncing = true
        state.error = nil
        do {
            let bridge = self.bridge
            let depth = fetchDepth
            let timeout = fetchTimeoutSeconds
            try await offMain { try bridge.sync(repoID: repoID, depth: depth, timeoutSeconds: timeout) }
        } catch {
            // The view already in state stays there. A failed refresh must not
            // cost you the data you had.
            state.syncing = false
            state.error = message(error)
            return
        }
        if let fresh = readView(repoID) {
            state.view = fresh
        }
        state.syncing = false
    }

    /// The view on disk, or nil if there is not one yet.
    private func readView(_ repoID: String) -> Clarity_V1_View? {
        try? bridge.view(repoID: repoID, limit: commitLimit)
    }

    private func loadRepos() throws {
        state.repos = try bridge.listRepos().repos
    }

    /// Runs a local action, reporting whatever it throws.
    ///
    /// Everything the core raises is already written for a person to read — the
    /// registry's "paste the URL you would clone", git's own fetch diagnosis —
    /// so the message is passed through rather than replaced with a category.
    private func act(_ work: () throws -> Void) async {
        state.busy = true
        state.error = nil
        do {
            try work()
            state.busy = false
        } catch {
            state.busy = false
            state.error = message(error)
        }
    }

    /// Runs blocking work off the main thread. Only the fetch needs it: it is a
    /// real network round trip, and the others are a file read or a write.
    private nonisolated func offMain(_ work: @escaping () throws -> Void) async throws {
        try await Task.detached(priority: .userInitiated) { try work() }.value
    }

    private func message(_ error: Error) -> String {
        let text = (error as NSError).localizedDescription
        return text.isEmpty ? String(describing: error) : text
    }
}
