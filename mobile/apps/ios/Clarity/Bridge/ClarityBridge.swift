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
    func sync(repoID: String, depth: Int, timeoutSeconds: Int) throws

    /// Reads what the last `sync` fetched. Never touches the network.
    func view(repoID: String, limit: Int) throws -> Clarity_V1_View
}
