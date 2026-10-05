import Foundation

/// The app's state machine.
///
/// Holds no SwiftUI types and no bindings, so the whole of it is testable
/// against `FakeBridge`. The views read `state` and call these methods; they
/// make no decisions of their own.
///
/// Every method is `async` and completes before it returns, which is what makes
/// a test a sequence of statements rather than a wait for an expectation.
/// What a fetch said, when it did not succeed.
struct FetchError: LocalizedError {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}

@MainActor
final class ClarityModel: ObservableObject {
    @Published private(set) var state = AppState()

    private let bridge: ClarityBridge

    /// Epoch seconds. Injected rather than read, for the same reason every
    /// renderer in this project takes a `now`: a test that cannot move the
    /// clock cannot test anything that ticks.
    private let clock: () -> Int64

    /// How many commits a repository screen asks for.
    private let commitLimit = 200
    /// Zero lets the core pick its own fetch depth and timeout.
    private let fetchDepth = 0
    private let fetchTimeoutSeconds = 0
    /// How often the open repository re-fetches, like the TUI's watcher.
    /// The same five seconds refsource polls at, so the phone and the terminal
    /// are the same freshness rather than approximately so.
    private let refreshSeconds = 5

    /// The clock and the auto-refresh, running only while the UI is visible.
    private var pump: Task<Void, Never>?
    private var sinceFetch = 0

    /// The repository the connect flow is working on.
    ///
    /// Held here rather than read back off the list, which is ordered by the
    /// name a row shows: "the one just added" and "the last one" are the same
    /// repository only until somebody connects one whose name sorts early, and
    /// then trusting a host would hand the answer to a different repository
    /// entirely.
    private var connecting: String?

    /// Whether the clock is running. Observable so a test can say "and then the
    /// app went to the background" without waiting a real second to prove it.
    var isTicking: Bool { pump != nil }

    init(bridge: ClarityBridge, clock: @escaping () -> Int64 = { Int64(Date().timeIntervalSince1970) }) {
        self.bridge = bridge
        self.clock = clock
    }

    func start() async {
        await act { try self.loadRepos() }
        // Land in a repository rather than on a menu. There is nothing to read
        // on the menu, and the one thing the app exists to show is one commit
        // list — the same reason a chat app opens in a conversation.
        if state.selected == nil, let first = state.repos.first?.id {
            await open(first)
        }
    }

    /// Starts the clock and the automatic refresh. Call when the UI becomes
    /// visible, and `pause` when it stops being.
    func resume() {
        guard pump == nil else { return }
        sinceFetch = 0
        pump = Task { [weak self] in
            while !Task.isCancelled {
                await self?.beat()
                try? await Task.sleep(nanoseconds: 1_000_000_000)
            }
        }
    }

    /// One beat of the clock: advance the time every timer is drawn against,
    /// and every `refreshSeconds` beats, fetch.
    ///
    /// Separate from `resume` so it can be tested. A test that has to sleep a
    /// real thirty seconds to watch an automatic refresh is slow and flaky in
    /// equal measure, and the only thing `resume` adds on top of this is the
    /// sleep itself.
    func beat() async {
        state.nowSeconds = clock()
        sinceFetch += 1
        guard sinceFetch >= refreshSeconds else { return }
        sinceFetch = 0
        if let id = state.selected {
            await fetch(id, background: true)
        }
    }

    /// Stops both. Nothing ticks and nothing fetches while the app is in the
    /// background: a timer nobody can see is only spending battery, and a fetch
    /// nobody asked for is spending their data too.
    func pause() {
        pump?.cancel()
        pump = nil
    }

    /// Opens the connect flow, from the empty state or from the switcher.
    func showConnect() {
        state.overlay = .connect
        state.connect = .idle
        state.error = nil
    }

    func showSwitcher() {
        state.overlay = .switcher
        state.error = nil
    }

    func showKey() async {
        state.overlay = .key
        state.error = nil
        await loadKey()
    }

    /// Fetches the device key, generating it on first call. Asking for it is
    /// what creates it, so a user who never adds an ssh remote never needs one.
    func loadKey() async {
        await act {
            self.state.publicKey = try self.bridge.publicKey(comment: "clarity on ios")
        }
    }

    /// Closes whatever is over the repository, leaving the selection alone.
    func closeOverlay() {
        state.overlay = nil
        state.connect = .idle
        state.error = nil
    }

    /// Connects a repository: track it, then fetch it once.
    ///
    /// The two are one action from the user's side — a repository that was
    /// added but never reached is not connected — so a fetch that comes back
    /// asking about a host key, or refusing the device key, leaves the flow
    /// open on the screen that explains it.
    func connect(url: String, branch: String) async {
        state.connect = .working
        state.error = nil

        var added: String?
        do {
            let bridge = self.bridge
            added = try await offMain { try bridge.addRepo(url: url, branch: branch) }
        } catch {
            state.connect = .failed(message: message(error))
            return
        }
        connecting = added
        await act { try self.loadRepos() }
        if let added { await attempt(added) }
    }

    /// Records the host key the prompt showed, then picks up where it stopped.
    func trustHost() async {
        guard case let .askHost(key, _) = state.connect, let id = connecting else { return }
        state.connect = .working
        do {
            let bridge = self.bridge
            let host = key.host
            let fingerprint = key.fingerprint
            try await offMain { try bridge.trustHost(host: host, fingerprint: fingerprint) }
        } catch {
            state.connect = .failed(message: message(error))
            return
        }
        await attempt(id)
    }

    /// Tries again after the user has fixed something on the host.
    func retryConnect() async {
        guard let id = connecting else { return }
        state.connect = .working
        await attempt(id)
    }

    /// One fetch of a repository being connected, sorted into the screen it
    /// should leave behind.
    private func attempt(_ repoID: String) async {
        let result: Clarity_V1_SyncResult
        do {
            let bridge = self.bridge
            let depth = fetchDepth
            let timeout = fetchTimeoutSeconds
            result = try await offMain {
                try bridge.sync(repoID: repoID, depth: depth, timeoutSeconds: timeout)
            }
        } catch {
            state.connect = .failed(message: message(error))
            return
        }

        switch result.outcome {
        case .ok:
            // The key has now been accepted by a host at least once, which is
            // what folds the key card away on the next connect.
            state.connect = .idle
            state.overlay = nil
            state.keyHasConnected = true
            await open(repoID)
        case .hostKeyUnknown:
            state.connect = .askHost(key: result.hostKey, changed: false)
        case .hostKeyChanged:
            state.connect = .askHost(key: result.hostKey, changed: true)
        case .authDenied:
            state.connect = .denied(gitOutput: result.gitOutput)
        default:
            state.connect = .failed(message: result.message)
        }
    }

    /// Renames a repository on this device. A blank name clears the rename.
    func rename(_ repoID: String, to name: String) async {
        await act {
            try self.bridge.rename(repoID: repoID, name: name)
            try self.loadRepos()
        }
    }

    /// Changes which branch a repository watches, then re-reads it.
    ///
    /// The fetch is not optional: the view on screen is of the old branch, and
    /// leaving it there under a new branch's name would be the most confusing
    /// possible outcome.
    func changeBranch(_ repoID: String, to branch: String) async {
        await act {
            try self.bridge.setBranch(repoID: repoID, branch: branch)
            try self.loadRepos()
        }
        state.view = nil
        await fetch(repoID, background: false)
    }

    func removeRepo(_ repoID: String) async {
        await act {
            try self.bridge.removeRepo(repoID)
            try self.loadRepos()
        }
        if state.selected == repoID {
            // Falling back to whatever is left beats leaving the main surface
            // showing a repository that is no longer in the list.
            let next = state.repos.first?.id
            state.selected = nil
            state.view = nil
            if let next { await open(next) }
        }
    }

    /// Shows a repository: what is already on disk first, then a fetch.
    func select(_ repoID: String) async {
        guard state.selected != repoID else { return }
        await open(repoID)
    }

    /// Re-fetches the open repository.
    func refresh() async {
        guard let id = state.selected else { return }
        await fetch(id, background: false)
    }

    func dismissError() {
        state.error = nil
    }

    /// Formats a duration, for the timers the UI ticks between fetches.
    ///
    /// The view arrives with every duration preformatted, which is right for
    /// the instant it was built and wrong one second later. This is how a row
    /// stays honest without asking Go to walk the commit graph again.
    func elapsed(_ seconds: Int64) -> String {
        bridge.elapsed(seconds: max(0, seconds))
    }

    // MARK: - internals

    /// Opens a repository: what is already on disk first, then a fetch.
    ///
    /// The order is the point. A phone is offline often, and the local object
    /// store is what makes that survivable — showing the last fetch immediately
    /// beats a spinner over nothing.
    private func open(_ repoID: String) async {
        // The outgoing repository's commits go immediately. Leaving them up
        // while the next one loads shows one repository's work under another's
        // name, which is worse than showing nothing.
        state.selected = repoID
        state.view = nil
        state.error = nil

        // A repository that has never been fetched has no branch to resolve, so
        // this fails. That is the expected state on a first open, not something
        // to report: the fetch about to run is the answer to it, and if the
        // fetch fails too, its reason is the useful one.
        if let onDisk = readView(repoID) {
            state.view = onDisk
        }
        await fetch(repoID, background: false)
    }

    /// Fetches, then re-reads.
    ///
    /// A background fetch that fails says nothing when there is already a view
    /// on screen. The alternative is an error that reappears every thirty
    /// seconds for as long as you are on a train, over data that is perfectly
    /// readable — the failure is worth reporting when you asked for it, or when
    /// there is nothing behind it to read.
    private func fetch(_ repoID: String, background: Bool) async {
        state.syncing = true
        if !background { state.error = nil }
        do {
            let bridge = self.bridge
            let depth = fetchDepth
            let timeout = fetchTimeoutSeconds
            let result = try await offMain {
                try bridge.sync(repoID: repoID, depth: depth, timeoutSeconds: timeout)
            }
            if result.outcome != .ok {
                // Everything that is not success is reported the same way for
                // now; the screens that tell an unknown host from a refused key
                // arrive with the connect flow.
                throw FetchError(result.message)
            }
        } catch {
            state.syncing = false
            if !(background && state.view != nil) {
                state.error = message(error)
            }
            return
        }
        let fresh = readView(repoID)
        // Reading a view is also what records its verdict, so the list that
        // shows those verdicts has to be re-read afterwards. Without this the
        // switcher keeps saying "nothing reported" about a repository whose own
        // screen is showing a green tick.
        try? loadRepos()
        state.syncing = false
        // A fetch that finished after the user moved on belongs to a repository
        // that is no longer on screen.
        if state.selected == repoID, let fresh {
            state.view = fresh
        }
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
    private nonisolated func offMain<T>(_ work: @escaping () throws -> T) async throws -> T {
        try await Task.detached(priority: .userInitiated) { try work() }.value
    }

    private func message(_ error: Error) -> String {
        let text = (error as NSError).localizedDescription
        return text.isEmpty ? String(describing: error) : text
    }
}
