import SwiftUI
import UIKit

/**
 This device's public key, on its own.

 Reached from the switcher once there are repositories, and from the empty
 state's menu before there are — the key is the thing you need *before* the
 first connection, and the host wants it pasted in.
 */
struct KeyView: View {
    @ObservedObject var model: ClarityModel

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    ErrorBar(error: model.state.error) { model.dismissError() }
                    Text(
                        "Add this as a deploy key on a repository, or under your account's "
                            + "SSH keys, with read access. The private half never leaves this phone."
                    )
                    .font(Type.bodySmall)
                    .foregroundColor(Ink.dim)

                    KeyCardView(publicKey: model.state.publicKey, startOpen: true, collapsible: false)
                }
                .padding(pageMargin)
            }
            .background(Ink.bg)
            .navigationTitle("Device key")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    Button("Done") { model.closeOverlay() }.tint(Ink.blue)
                }
            }
        }
        .task { await model.loadKey() }
    }
}
