import Foundation
@testable import Clarity

/// A bridge with no Go behind it.
///
/// Every failure the real core can hand back is reachable by setting a
/// property, so the model's behaviour on a rejected URL or a dead connection is
/// a unit test rather than something you find out about on a train.
final class FakeBridge: ClarityBridge {

    /// Method names in call order, for asserting what ran and in what order.
    var calls: [String] = []

    /// The repositories "on disk".
    var repos: [Clarity_V1_RepoSummary] = []

    /// Views keyed by repo id. A missing entry behaves like a never-synced repo.
    var views: [String: Clarity_V1_View] = [:]

    var keyExists = false
    var key = "ssh-ed25519 AAAAC3Nz fake"

    /// Set to make the matching call throw with this message.
    var failAddRepo: String?
    var failSync: String?
    var failList: String?

    /// Runs inside `sync`, before it succeeds or fails. A test uses it to make a
    /// fetch deliver new commits.
    var onSync: (String) -> Void = { _ in }

    func hasKey() -> Bool {
        calls.append("hasKey")
        return keyExists
    }

    func publicKey(comment: String) throws -> String {
        calls.append("publicKey")
        keyExists = true
        return "\(key) \(comment)"
    }

    func addRepo(url: String, branch: String) throws -> String {
        calls.append("addRepo")
        if let failAddRepo { throw FakeError(failAddRepo) }
        var summary = Clarity_V1_RepoSummary()
        summary.id = "id-\(repos.count + 1)"
        summary.url = url
        summary.name = (url as NSString).lastPathComponent.replacingOccurrences(of: ".git", with: "")
        summary.branch = branch.isEmpty ? "main" : branch
        repos.append(summary)
        return summary.id
    }

    func removeRepo(_ repoID: String) throws {
        calls.append("removeRepo")
        repos.removeAll { $0.id == repoID }
        views[repoID] = nil
    }

    func listRepos() throws -> Clarity_V1_RepoList {
        calls.append("listRepos")
        if let failList { throw FakeError(failList) }
        var list = Clarity_V1_RepoList()
        list.repos = repos
        return list
    }

    func sync(repoID: String, depth: Int, timeoutSeconds: Int) throws {
        calls.append("sync")
        onSync(repoID)
        if let failSync { throw FakeError(failSync) }
    }

    // Not the real formatter — the model only passes through to it, so a test
    // that asserted the wording would be testing Go through two layers.
    func elapsed(seconds: Int64) -> String { "\(seconds)s" }

    func view(repoID: String, limit: Int) throws -> Clarity_V1_View {
        calls.append("view")
        guard let view = views[repoID] else {
            // What a repo that has never been fetched gives you: go-git has no
            // branch to resolve, so the read fails rather than returning empty.
            throw FakeError("reference not found")
        }
        return view
    }

    /// A one-commit view, enough to tell "we have data" from "we do not".
    static func view(of subject: String) -> Clarity_V1_View {
        var commit = Clarity_V1_Commit()
        commit.subject = subject
        commit.sha = "abcdef1234567890"
        var section = Clarity_V1_Section()
        section.kind = .head
        section.label = "HEAD"
        section.commits = [commit]
        var flow = Clarity_V1_Flow()
        flow.name = "deploy"
        flow.sections = [section]
        var view = Clarity_V1_View()
        view.flows = [flow]
        return view
    }
}

struct FakeError: LocalizedError {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}
