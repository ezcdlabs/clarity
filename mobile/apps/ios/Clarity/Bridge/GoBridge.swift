import ClarityCore
import Foundation

/// The real bridge: gomobile's generated client, with protobuf bytes decoded on
/// the way back.
///
/// There is no logic here beyond that decoding. Anything that needed a decision
/// would be untestable at this layer, so it lives in Go — where
/// `mobile/internal` tests cover it — or in the model, where a fake covers it.
struct GoBridge: ClarityBridge {
    private let client: ClarityCoreClient

    /// Opens a bridge over the directory iOS gives us for private data.
    init(dataDir: URL) throws {
        try FileManager.default.createDirectory(at: dataDir, withIntermediateDirectories: true)
        var error: NSError?
        guard let client = ClarityCoreNew(dataDir.path, &error) else {
            throw error ?? BridgeError.unavailable
        }
        self.client = client
    }

    func hasKey() -> Bool { client.hasKey() }

    // These two take an NSError out-parameter rather than arriving as throwing
    // calls. Swift only applies the error convention when a failure can be told
    // from the return value, and gomobile declares both of these _Nonnull — so
    // the error is the only thing that says anything went wrong, and an empty
    // string means nothing at all.
    func publicKey(comment: String) throws -> String {
        var failure: NSError?
        let key = client.publicKey(comment, error: &failure)
        if let failure { throw failure }
        return key
    }

    func keyFingerprint() throws -> String {
        var failure: NSError?
        let fp = client.keyFingerprint(&failure)
        if let failure { throw failure }
        return fp
    }

    func addRepo(url: String, branch: String) throws -> String {
        var failure: NSError?
        let id = client.addRepo(url, branch: branch, error: &failure)
        if let failure { throw failure }
        return id
    }

    func removeRepo(_ repoID: String) throws {
        // The core deliberately leaves the objects behind rather than deleting
        // user data as a side effect of forgetting a name. Someone has to, and
        // the app is the side that knows the repo is really gone.
        let store = client.storePath(repoID)
        try client.removeRepo(repoID)
        try? FileManager.default.removeItem(atPath: store)
    }

    func listRepos() throws -> Clarity_V1_RepoList {
        try Clarity_V1_RepoList(serializedBytes: client.listRepos())
    }

    func sync(repoID: String, depth: Int, timeoutSeconds: Int) throws -> Clarity_V1_SyncResult {
        try Clarity_V1_SyncResult(
            serializedBytes: client.sync(repoID, depth: depth, timeoutSeconds: timeoutSeconds)
        )
    }

    func trustHost(host: String, fingerprint: String) throws {
        try client.trustHost(host, fingerprint: fingerprint)
    }

    func rename(repoID: String, name: String) throws {
        try client.rename(repoID, name: name)
    }

    func view(repoID: String, limit: Int) throws -> Clarity_V1_View {
        try Clarity_V1_View(serializedBytes: client.view(repoID, limit: limit))
    }

    func elapsed(seconds: Int64) -> String { ClarityCoreElapsed(seconds) }
}

enum BridgeError: Error {
    case unavailable
}
