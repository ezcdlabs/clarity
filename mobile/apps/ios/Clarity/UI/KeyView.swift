import SwiftUI
import UIKit

/// This device's public key, for pasting into whichever host the user uses.
///
/// There is no provider integration and no OAuth app, because clarity is not a
/// GitHub tool — a key the user installs themselves works on GitHub, GitLab and
/// a box in a cupboard equally.
struct KeyView: View {
    @ObservedObject var model: ClarityModel

    var body: some View {
        VStack(spacing: 0) {
            TopBar(title: "Device key", leading: { BackArrow { model.closeOverlay() } }, trailing: { EmptyView() })
            ErrorBar(error: model.state.error) { model.dismissError() }

            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    Text("Add this as a deploy key or an account key on the host, with read "
                        + "access. The private half never leaves this device.")
                        .font(.system(size: 13)).foregroundColor(Ink.dim)

                    if let key = model.state.publicKey {
                        Text(key)
                            .font(.system(size: 12, design: .monospaced))
                            .foregroundColor(Ink.text)
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(12)
                            .background(Ink.surface)

                        Button("Copy") { UIPasteboard.general.string = key }
                            .foregroundColor(Ink.blue)
                    } else {
                        Text("Generating…").font(.system(size: 13)).foregroundColor(Ink.dim)
                    }
                }
                .padding(16)
            }
        }
    }
}
