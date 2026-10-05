import Foundation

/// A screen presented over the repository, rather than beside it.
///
/// Identified by its own value, which is what lets one sheet stand for all
/// three and swap between them: the identity changing is the swap. Hence
/// Hashable rather than merely Equatable — an id has to be hashable.
enum Overlay: Hashable, Identifiable {
    var id: Self { self }

    /// Paste a clone address and connect. One flow, presented as a sheet.
    case connect
    /// This device's public key, on its own.
    case key
    /// The list of repositories.
    case switcher
}

/// Where the connect flow has got to.
///
/// States rather than a pair of booleans: they are genuinely exclusive, each
/// draws a different screen, and two of them carry something the screen cannot
/// work out for itself — a fingerprint to show, or the raw thing git said.
enum Connect: Equatable {
    case idle
    case working
    /// The host is not one this device has agreed to. Ask, then trust.
    case askHost(key: Clarity_V1_HostKey, changed: Bool)
    /// The host refused the device key. Fixable, with its own screen.
    case denied(gitOutput: String)
    case failed(message: String)
}

/// The whole of what the UI draws, in one value.
///
/// `selected` outlives everything presented over it. The repository is the app;
/// the switcher, the connect flow and the key screen happen in front of it, and
/// dismissing one puts you back where you were.
struct AppState: Equatable {
    var repos: [Clarity_V1_RepoSummary] = []
    var selected: String?
    var view: Clarity_V1_View?
    var overlay: Overlay?
    var connect: Connect = .idle
    /// True while a network fetch is in flight.
    var syncing = false
    /// True while a local action is in flight.
    var busy = false
    var publicKey: String?
    /// Whether any repository has ever connected with this device's key. It
    /// decides whether the connect screen opens with the key on show.
    var keyHasConnected = false
    var error: String?
    /// Epoch seconds, advanced while the UI is visible. Every timer is drawn
    /// against this, so they tick together and none tick unobserved.
    var nowSeconds: Int64 = 0

    var repo: Clarity_V1_RepoSummary? {
        repos.first { $0.id == selected }
    }

    /// True when there is nothing to show yet, which is its own screen.
    var isEmpty: Bool { repos.isEmpty }
}

extension Clarity_V1_RepoSummary {
    /// What this repository is called: the rename if there is one.
    var title: String { alias.isEmpty ? name : alias }

    /// The dimmed prefix before the title, with its trailing slash.
    ///
    /// A renamed repository has none: the alias replaces the whole label and
    /// the derived namespace/name moves to the subtitle, so a namespace in
    /// front of a chosen name would attach itself to the wrong thing.
    var titlePrefix: String {
        alias.isEmpty && !namespace.isEmpty ? "\(namespace)/" : ""
    }

    /// The line under the title: what it is, where it came from.
    var subtitle: String {
        var parts: [String] = []
        if !alias.isEmpty {
            parts.append(namespace.isEmpty ? name : "\(namespace)/\(name)")
        }
        parts.append(branch)
        if !host.isEmpty { parts.append(host) }
        return parts.joined(separator: " · ")
    }

    /// Whether anything in this repository is currently failing.
    var isBroken: Bool {
        ci == .failed || flows.contains { $0.deploy == .failed }
    }
}
