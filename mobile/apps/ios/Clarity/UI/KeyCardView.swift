import SwiftUI
import UIKit

/**
 This device's public key, with what to do with it.

 Shared between the connect flow and the key screen, because they are the same
 card with a different reason for being on screen.

 `startOpen` is false once any repository has connected with this key: the first
 time you need the whole thing in front of you, and after that it is a fact you
 occasionally check.
 */
struct KeyCardView: View {
    let publicKey: String?
    var startOpen: Bool
    var collapsible = true

    @State private var open: Bool

    init(publicKey: String?, startOpen: Bool, collapsible: Bool = true) {
        self.publicKey = publicKey
        self.startOpen = startOpen
        self.collapsible = collapsible
        _open = State(initialValue: startOpen || !collapsible)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Button {
                if collapsible { withAnimation { open.toggle() } }
            } label: {
                HStack(spacing: 10) {
                    Image(systemName: "key").font(.system(size: 15)).foregroundColor(Ink.dim)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(open ? "This device's key" : "Connects with this device's key")
                            .font(Type.cardTitle)
                            .foregroundColor(Ink.text)
                        if !open {
                            // The kind of key, and nothing else. A fingerprint
                            // here could only be a fragment, and a hash you
                            // cannot compare is the shape of a fact rather than
                            // one.
                            Text("ed25519").font(Type.mono).foregroundColor(Ink.dim)
                        }
                    }
                    Spacer()
                    if collapsible {
                        Image(systemName: open ? "chevron.up" : "chevron.down")
                            .font(.system(size: 13))
                            .foregroundColor(Ink.dim)
                    }
                }
            }
            .buttonStyle(.plain)
            .disabled(!collapsible)

            if open {
                Text(
                    "Add it on your git host first, either as a read-only deploy key "
                        + "on the repository or under your account's SSH keys."
                )
                .font(Type.supporting)
                .foregroundColor(Ink.dim)

                Text(publicKey ?? "Generating…")
                    .font(Type.mono)
                    .foregroundColor(Ink.text)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                    .background(Ink.bg)
                    .clipShape(RoundedRectangle(cornerRadius: 12))

                HStack {
                    Spacer()
                    Button {
                        if let publicKey { UIPasteboard.general.string = publicKey }
                    } label: {
                        Label("Copy key", systemImage: "doc.on.doc").font(Type.bodySmall)
                    }
                    .buttonStyle(.bordered)
                    .tint(Ink.blue)
                    .disabled(publicKey == nil)
                }
            }
        }
        .padding(16)
        .background(Ink.surface)
        .clipShape(RoundedRectangle(cornerRadius: 20))
    }
}
