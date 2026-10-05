import Foundation

/// Everything the app can ask of the Go core.
///
/// The UI never touches the generated bindings directly: they need a device and
/// a bound framework, and nothing that depends on them can be tested without
/// one. Behind this sits either `GoBridge` or `FakeBridge`, and the screens
/// cannot tell which.
///
/// Every method blocks. The core does network and disk work synchronously on
/// purpose — which thread that happens on is the model's decision, not the
/// bridge's.
protocol ClarityBridge {
    /// Whether a device key exists yet, without creating one.
    func hasKey() -> Bool

    /// The `authorized_keys` line for this device, generating the key if needed.
    func publicKey(comment: String) throws -> String

    /// Tracks a repository, returning its id. The URL is the one you'd clone.
    func addRepo(url: String, branch: String) throws -> String

    /// Forgets a repository and reclaims its object store.
    func removeRepo(_ repoID: String) throws

    func listRepos() throws -> Clarity_V1_RepoList

    /// Fetches a repository. Blocking, and the only method that uses network.
    ///
    /// Returns what the fetch did rather than throwing, because a fetch has
    /// failures the UI must tell apart: an unknown host is a question with its
    /// own dialog, a rejected key is a fixable setup step with its own screen,
    /// and everything else is a message.
    func sync(repoID: String, depth: Int, timeoutSeconds: Int) throws -> Clarity_V1_SyncResult

    /// Records a host key the user agreed to, so a refused fetch can retry.
    func trustHost(host: String, fingerprint: String) throws

    /// Renames a repository on this device. A blank name clears the rename.
    func rename(repoID: String, name: String) throws

    /// Reads what the last `sync` fetched. Never touches the network.
    func view(repoID: String, limit: Int) throws -> Clarity_V1_View

    /// Formats a duration the way every clarity UI formats one.
    ///
    /// Here rather than in Swift because a running timer is recomputed every
    /// second on the device, and the alternative to asking Go was
    /// reimplementing the rule — one more place for "3m 29s" to drift into
    /// "3:29".
    func elapsed(seconds: Int64) -> String
}
