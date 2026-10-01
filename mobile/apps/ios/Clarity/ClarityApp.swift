import SwiftUI

@main
@MainActor
struct ClarityApp: App {
    @StateObject private var model = ClarityApp.makeModel()

    var body: some Scene {
        WindowGroup {
            RootView(model: model)
                .task { await model.start() }
        }
    }

    /// One client per app, not one per screen: the key, the registry and every
    /// object store live under a single directory, and two clients over the same
    /// directory would be two writers.
    private static func makeModel() -> ClarityModel {
        let dir = URL.applicationSupport.appendingPathComponent("clarity", isDirectory: true)
        do {
            return ClarityModel(bridge: try GoBridge(dataDir: dir))
        } catch {
            // Nothing can work without a client, and there is no second thing to
            // try — the directory iOS gave us is unusable. Crashing here reports
            // it, where a silent empty menu would not.
            fatalError("could not open the clarity core: \(error)")
        }
    }
}

private extension URL {
    static var applicationSupport: URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
    }
}
