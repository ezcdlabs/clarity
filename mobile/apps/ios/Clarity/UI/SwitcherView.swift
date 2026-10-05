import SwiftUI

/**
 The repository switcher.

 A `List` in a sheet rather than Android's hand-laid rows: on iOS a list of
 things you pick from is a List, and getting it for free also gets swipe actions
 — which is how a destructive row action is reached on this platform. Android
 puts remove behind a long press; here that would be the ported answer rather
 than the native one.
 */
struct SwitcherView: View {
    @ObservedObject var model: ClarityModel

    @State private var renaming: Clarity_V1_RepoSummary?
    @State private var changingBranch: Clarity_V1_RepoSummary?
    @State private var removing: Clarity_V1_RepoSummary?

    var body: some View {
        NavigationStack {
            List {
                Section {
                    ForEach(model.state.repos, id: \.id) { repo in
                        Button {
                            Task {
                                model.closeOverlay()
                                await model.select(repo.id)
                            }
                        } label: {
                            row(repo)
                        }
                        .listRowBackground(repo.id == model.state.selected ? Ink.line : Ink.surface)
                        .swipeActions {
                            Button("Remove", role: .destructive) { removing = repo }
                            Button("Rename") { renaming = repo }.tint(Ink.blue)
                        }
                        .contextMenu {
                            Button { renaming = repo } label: { Label("Rename…", systemImage: "pencil") }
                            Button { changingBranch = repo } label: {
                                Label("Change branch…", systemImage: "arrow.triangle.branch")
                            }
                            Button(role: .destructive) { removing = repo } label: {
                                Label("Remove", systemImage: "trash")
                            }
                        }
                    }
                }

                Section {
                    Button {
                        Task { await model.showKey() }
                    } label: {
                        Label("Device key", systemImage: "key")
                            .font(Type.bodySmall)
                            .foregroundColor(Ink.text)
                    }
                    .listRowBackground(Ink.surface)
                }
            }
            .listStyle(.insetGrouped)
            .scrollContentBackground(.hidden)
            .background(Ink.bg)
            .navigationTitle("Repositories")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    Button {
                        model.showConnect()
                    } label: {
                        // The glyph alone: a `Label` in a navigation bar draws
                        // its title too, and "Add" beside a plus says nothing
                        // the plus has not already said.
                        Image(systemName: "plus")
                    }
                    .tint(Ink.blue)
                    .accessibilityLabel("Add a repository")
                }
            }
        }
        .sheet(item: $renaming) { RenameSheet(repo: $0, model: model) }
        .sheet(item: $changingBranch) { BranchSheet(repo: $0, model: model) }
        .alert(
            "Remove \(removing?.title ?? "")?",
            isPresented: presenting($removing),
            presenting: removing
        ) { repo in
            Button("Remove", role: .destructive) {
                Task { await model.removeRepo(repo.id) }
            }
            Button("Cancel", role: .cancel) {}
        } message: { _ in
            Text(removalWarning)
        }
    }

    private func row(_ repo: Clarity_V1_RepoSummary) -> some View {
        HStack(spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 0) {
                    if !repo.titlePrefix.isEmpty {
                        // The namespace yields first; the name never truncates.
                        Text(repo.titlePrefix)
                            .font(Type.cardTitle.weight(.regular))
                            .foregroundColor(Ink.dim)
                            .lineLimit(1)
                            .truncationMode(.head)
                            .layoutPriority(0)
                    }
                    Text(repo.title)
                        .font(Type.cardTitle)
                        .foregroundColor(repo.isBroken ? Ink.red : Ink.text)
                        .lineLimit(1)
                        .layoutPriority(1)
                }
                Text(repo.subtitle).font(Type.mono).foregroundColor(Ink.dim).lineLimit(1)
            }
            Spacer(minLength: 8)
            status(repo)
        }
    }

    /// How a repository is doing, in the space a list row has.
    ///
    /// One mark per flow and no names: a flow is never hidden, because the
    /// stuck one is the one worth seeing and would be the one dropped. The
    /// description VoiceOver gets does name them — it has no width to run out
    /// of.
    private func status(_ repo: Clarity_V1_RepoSummary) -> some View {
        var spoken = "ci \(describe(repo.ci))"
        for flow in repo.flows { spoken += ", \(flow.name) \(describe(flow.deploy))" }

        return HStack(spacing: 6) {
            Text("ci").font(Type.monoMeta).foregroundColor(Ink.dim)
            StatusGlyph(status: repo.ci, prominent: true, size: 14)
            Text("deploy").font(Type.monoMeta).foregroundColor(Ink.dim).padding(.leading, 2)
            if repo.flows.isEmpty {
                StatusGlyph(status: repo.deploy, prominent: true, size: 14)
            } else {
                ForEach(Array(repo.flows.enumerated()), id: \.offset) { _, flow in
                    StatusGlyph(status: flow.deploy, prominent: true, size: 14)
                }
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spoken)
    }

    private func describe(_ status: Clarity_V1_Status) -> String {
        switch status {
        case .passed: return "passed"
        case .failed: return "failed"
        case .started: return "in progress"
        default: return "none"
        }
    }

}

/// What removing actually does, written once because both the switcher's swipe
/// and the repository menu's item ask it. The destructive part is local, and
/// the part a user would worry about — access to the host — is untouched.
let removalWarning =
    "Clarity stops watching it and deletes its local copy from this phone. "
    + "Nothing changes on the host, and the device key stays authorised until "
    + "you remove it there."

/// Drives an alert from an optional value.
///
/// `alert(isPresented:presenting:)` wants a boolean and the value separately,
/// and the one piece of state worth keeping is the value — a second boolean
/// alongside it is a second thing that can disagree.
func presenting<T>(_ value: Binding<T?>) -> Binding<Bool> {
    Binding(
        get: { value.wrappedValue != nil },
        set: { shown in if !shown { value.wrappedValue = nil } }
    )
}

/// Renaming, which is local to this phone and clearable.
struct RenameSheet: View {
    let repo: Clarity_V1_RepoSummary
    @ObservedObject var model: ClarityModel
    @Environment(\.dismiss) private var dismiss
    @State private var name: String = ""

    private var derived: String {
        repo.namespace.isEmpty ? repo.name : "\(repo.namespace)/\(repo.name)"
    }

    var body: some View {
        NavigationStack {
            Form {
                TextField("Display name", text: $name)
                    .submitLabel(.done)
                Text("Only on this phone. Leave it empty to go back to \(derived).")
                    .font(Type.supporting)
                    .foregroundColor(Ink.dim)
            }
            .navigationTitle("Rename")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarLeading) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .navigationBarTrailing) {
                    Button("Save") {
                        Task { await model.rename(repo.id, to: name) }
                        dismiss()
                    }
                }
            }
        }
        .presentationDetents([.height(220)])
        .onAppear { name = repo.alias }
    }
}

/// Changing which branch a repository watches.
struct BranchSheet: View {
    let repo: Clarity_V1_RepoSummary
    @ObservedObject var model: ClarityModel
    @Environment(\.dismiss) private var dismiss
    @State private var branch: String = ""

    var body: some View {
        NavigationStack {
            Form {
                TextField("Branch", text: $branch)
                    .font(Type.mono)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .submitLabel(.done)
                Text("Clarity will fetch this branch instead. Leave it empty for main.")
                    .font(Type.supporting)
                    .foregroundColor(Ink.dim)
            }
            .navigationTitle("Change branch")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarLeading) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .navigationBarTrailing) {
                    Button("Save") {
                        Task { await model.changeBranch(repo.id, to: branch) }
                        dismiss()
                    }
                }
            }
        }
        .presentationDetents([.height(220)])
        .onAppear { branch = repo.branch }
    }
}

/// `sheet(item:)` needs an identity for the thing it presents, and a `List`
/// needs one for a row.
extension Clarity_V1_RepoSummary: Identifiable {}
