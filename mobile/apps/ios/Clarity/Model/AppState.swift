import Foundation

/// Which screen is in front of the user.
enum Screen: Equatable {
    /// The menu of repositories that have been added.
    case repos
    /// Paste a clone URL.
    case addRepo
    /// The device's public key, for pasting into a host.
    case key
    /// One repository's commits.
    case repo(id: String)
}

/// The whole of what the UI draws, in one value.
///
/// `view` is deliberately kept across a failed refresh: a phone loses its
/// connection constantly, and yesterday's pipeline is more use than an empty
/// screen with an error on it.
struct AppState: Equatable {
    var screen: Screen = .repos
    var repos: [Clarity_V1_RepoSummary] = []
    var view: Clarity_V1_View?
    /// True while a network fetch is in flight.
    var syncing = false
    /// True while a local action (add, remove, key generation) is in flight.
    var busy = false
    var publicKey: String?
    var error: String?

    var repo: Clarity_V1_RepoSummary? {
        guard case let .repo(id) = screen else { return nil }
        return repos.first { $0.id == id }
    }
}
