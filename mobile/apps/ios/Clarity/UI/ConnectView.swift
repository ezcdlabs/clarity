import SwiftUI

/**
 Connecting a repository: one flow rather than a form and a surprise.

 Everything that can go wrong on a first connection is a step on the way rather
 than an error thrown back at the field — an unknown host is a question with an
 answer, and a refused key is a thing to go and fix with the key right there to
 copy.
 */
struct ConnectView: View {
    @ObservedObject var model: ClarityModel

    @State private var url = ""
    @State private var branch = ""

    private var working: Bool { model.state.connect == .working }

    private var denied: String? {
        if case let .denied(output) = model.state.connect { return output }
        return nil
    }

    private var failure: String? {
        if case let .failed(message) = model.state.connect { return message }
        return nil
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    Text("Clarity reads it over SSH, like git fetch.")
                        .font(Type.bodySmall)
                        .foregroundColor(Ink.dim)

                    VStack(spacing: 12) {
                        field("SSH clone address", text: $url, mono: true)
                            .keyboardType(.URL)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .submitLabel(.go)
                            .overlay(alignment: .trailing) {
                                if denied != nil {
                                    Image(systemName: "exclamationmark.circle.fill")
                                        .foregroundColor(Ink.red)
                                        .padding(.trailing, 12)
                                }
                            }
                        HStack(spacing: 10) {
                            Image(systemName: "arrow.triangle.branch").foregroundColor(Ink.dim)
                            field("main", text: $branch, mono: true)
                            Text("branch").font(Type.supporting).foregroundColor(Ink.dim)
                        }
                        .padding(.horizontal, 12)
                        .padding(.vertical, 4)
                        .overlay(
                            RoundedRectangle(cornerRadius: 12).stroke(Ink.line, lineWidth: 1)
                        )
                    }
                    .disabled(working)

                    if working { progress }
                    if let denied { deniedCard(denied) }
                    if let failure {
                        Text(failure).font(Type.supporting).foregroundColor(Ink.red)
                    }

                    if !working {
                        KeyCardView(
                            publicKey: model.state.publicKey,
                            // Forced open when the host has just refused it:
                            // that is the moment the key is the whole answer.
                            startOpen: denied != nil || !model.state.keyHasConnected,
                            collapsible: denied == nil
                        )
                    }

                    PrimaryButton(
                        title: working ? "Connecting…" : (denied != nil ? "Try again" : "Connect"),
                        systemImage: denied != nil ? "arrow.clockwise" : nil,
                        enabled: !url.isEmpty,
                        working: working
                    ) {
                        Task {
                            if denied != nil {
                                await model.retryConnect()
                            } else {
                                await model.connect(url: url, branch: branch)
                            }
                        }
                    }
                }
                .padding(pageMargin)
            }
            .background(Ink.bg)
            .navigationTitle("Connect a repository")
            .navigationBarTitleDisplayMode(.large)
            .toolbar {
                ToolbarItem(placement: .navigationBarLeading) {
                    // Live while connecting: a fetch going nowhere is exactly
                    // when somebody wants out.
                    Button("Cancel") { model.closeOverlay() }.tint(Ink.dim)
                }
            }
        }
        .task { await model.loadKey() }
        .sheet(isPresented: askingHost) { HostKeySheet(model: model) }
    }

    private var askingHost: Binding<Bool> {
        Binding(
            get: { if case .askHost = model.state.connect { return true } else { return false } },
            set: { shown in if !shown { model.closeOverlay() } },
        )
    }

    private func field(_ placeholder: String, text: Binding<String>, mono: Bool) -> some View {
        TextField(placeholder, text: text)
            .font(mono ? Type.mono : Type.body)
            .foregroundColor(Ink.text)
            .textFieldStyle(.plain)
            .padding(.vertical, 12)
            .padding(.horizontal, 12)
            .background(Ink.surface)
            .clipShape(RoundedRectangle(cornerRadius: 12))
    }

    /// What is happening, while it happens.
    ///
    /// The core reports one thing — reached, or not — so this says that one
    /// thing rather than inventing stages it cannot observe. A progress list
    /// measuring nothing is a lie that gets found out the first time something
    /// hangs.
    private var progress: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("CONNECTING").font(Type.overline).tracking(1.2).foregroundColor(Ink.dim)
            Text(
                "Reading the branch and clarity's events over SSH. The first fetch "
                    + "reads the recent history; later ones only fetch what changed."
            )
            .font(Type.supporting)
            .foregroundColor(Ink.dim)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(Ink.surface)
        .clipShape(RoundedRectangle(cornerRadius: 20))
    }

    /// The host refused the key. Fixable, and the fix is on this screen.
    private func deniedCard(_ gitOutput: String) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("The host didn't accept this device's key", systemImage: "lock")
                .font(Type.cardTitle)
                .foregroundColor(Ink.text)
            Text(
                "Add the key below to the repository's deploy keys — read access is "
                    + "enough — then try again."
            )
            .font(Type.supporting)
            .foregroundColor(Ink.dim)

            if !gitOutput.isEmpty {
                DisclosureGroup {
                    // Git's own words, not a summary: whoever is debugging a
                    // key wants what the host actually said.
                    Text(gitOutput)
                        .font(Type.mono)
                        .foregroundColor(Ink.dim)
                        .frame(maxWidth: .infinity, alignment: .leading)
                } label: {
                    Label("Show git output", systemImage: "terminal")
                        .font(Type.supporting)
                        .foregroundColor(Ink.blue)
                }
                .tint(Ink.blue)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(Ink.errorBg)
        .clipShape(RoundedRectangle(cornerRadius: 20))
    }
}

/**
 The first connection to a host.

 A sheet rather than an alert, which is where this diverges from Android on
 purpose: iOS alerts take a title, a message and buttons, and this one has a
 fingerprint block that has to be read character by character. An alert would
 have to flatten it into prose.
 */
struct HostKeySheet: View {
    @ObservedObject var model: ClarityModel

    private var asking: (key: Clarity_V1_HostKey, changed: Bool)? {
        if case let .askHost(key, changed) = model.state.connect { return (key, changed) }
        return nil
    }

    var body: some View {
        if let asking {
            NavigationStack {
                VStack(alignment: .leading, spacing: 14) {
                    Image(systemName: "touchid")
                        .font(.system(size: 32))
                        .foregroundColor(asking.changed ? Ink.red : Ink.blue)

                    Text(asking.changed ? "Host key changed" : "Trust this host?")
                        .font(Type.dialogTitle)
                        .foregroundColor(asking.changed ? Ink.red : Ink.text)

                    Text(
                        asking.changed
                            ? "The key \(asking.key.host) is presenting is not the one this "
                                + "device trusted before. That can mean the host was rebuilt — "
                                + "or that something is between you and it."
                            : "This is the first connection to \(asking.key.host). Check that "
                                + "this fingerprint matches the one your host publishes."
                    )
                    .font(Type.bodySmall)
                    .foregroundColor(Ink.dim)

                    VStack(alignment: .leading, spacing: 4) {
                        Text(asking.key.type).font(Type.overline).foregroundColor(Ink.dim)
                        Text(asking.key.fingerprint)
                            .font(Type.mono)
                            .foregroundColor(Ink.text)
                            .textSelection(.enabled)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                    .background(Ink.bg)
                    .clipShape(RoundedRectangle(cornerRadius: 12))

                    Text("Clarity remembers it. If it ever changes, you'll be asked again.")
                        .font(Type.supporting)
                        .foregroundColor(Ink.dim)

                    PrimaryButton(title: "Trust and continue") {
                        Task { await model.trustHost() }
                    }
                    .padding(.top, 6)

                    Spacer()
                }
                .padding(pageMargin)
                .background(Ink.surface)
                .toolbar {
                    ToolbarItem(placement: .navigationBarLeading) {
                        Button("Cancel") { model.closeOverlay() }.tint(Ink.dim)
                    }
                }
            }
            .presentationDetents([.medium])
        }
    }
}
