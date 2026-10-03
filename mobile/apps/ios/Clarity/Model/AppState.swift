import Foundation

/// A screen that sits over the repository, rather than replacing it.
enum Overlay: Equatable {
    /// Paste a clone URL.
    case addRepo
    /// The device's public key, for pasting into a host.
    case key
}

/// The whole of what the UI draws, in one value.
///
/// `selected` outlives the page you are on. The repository list is the page
/// beside the repository, not a destination you travel to — the way Slack puts
/// its channel list beside the conversation — so sliding over and back puts you
/// where you were rather than nowhere.
///
/// `view` is deliberately kept across a failed refresh: a phone loses its
/// connection constantly, and yesterday's pipeline is more use than an empty
/// screen with an error on it.
struct AppState: Equatable {
    var repos: [Clarity_V1_RepoSummary] = []
    var selected: String?
    var view: Clarity_V1_View?
    var overlay: Overlay?
    /// True while a network fetch is in flight.
    var syncing = false
    /// True while a local action (add, remove, key generation) is in flight.
    var busy = false
    var publicKey: String?
    var error: String?
    /// Epoch seconds, advanced by the model while the UI is visible. Every
    /// timer on screen is formatted against this, so they all tick together and
    /// none of them tick while nobody is looking.
    var nowSeconds: Int64 = 0

    var repo: Clarity_V1_RepoSummary? {
        repos.first { $0.id == selected }
    }
}
