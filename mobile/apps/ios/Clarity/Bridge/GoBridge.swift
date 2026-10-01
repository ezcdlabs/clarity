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

    func publicKey(comment: String) throws -> String {
        try client.publicKey(comment)
    }

    func addRepo(url: String, branch: String) throws -> String {
        try client.addRepo(url, branch: branch)
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

    func sync(repoID: String, depth: Int, timeoutSeconds: Int) throws {
        try client.sync(repoID, depth: depth, timeoutSeconds: timeoutSeconds)
    }

    func view(repoID: String, limit: Int) throws -> Clarity_V1_View {
        try Clarity_V1_View(serializedBytes: client.view(repoID, limit: limit))
    }
}

enum BridgeError: Error {
    case unavailable
}
