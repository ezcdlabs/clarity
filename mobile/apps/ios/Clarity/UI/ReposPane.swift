import SwiftUI

/// The left page: every tracked repository, with the open one marked.
///
/// Picking one only changes the selection. Sliding to the repository is the
/// pager's job, triggered by the selection changing — so the first launch and
/// the fallback after a removal carry you across too, not just a tap.
struct ReposPane: View {
    @ObservedObject var model: ClarityModel

    var body: some View {
        VStack(spacing: 0) {
            TopBar(title: "clarity") {
                HStack(spacing: 8) {
                    GlyphButton("plus", "Add a repository", tint: Ink.blue) { model.showAddRepo() }
                    // The device key is a once-ever action, so it goes where
                    // once-ever actions go rather than taking a seat in the bar.
                    Menu {
                        Button("Device key") { Task { await model.showKey() } }
                    } label: {
                        Image(systemName: "ellipsis")
                            .font(.system(size: 17))
                            .foregroundColor(Ink.dim)
                    }
                    .accessibilityLabel("More")
                }
            }

            Sheet {
            VStack(spacing: 0) {
            if model.state.repos.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("No repositories yet.")
                        .font(.system(size: 15)).foregroundColor(Ink.text)
                    Text("Add one by pasting the URL you would clone. If it is an ssh URL, "
                        + "put this device's key on the host first.")
                        .font(.system(size: 13)).foregroundColor(Ink.dim)
                }
                .padding(16)
                Spacer()
            } else {
                List {
                    ForEach(model.state.repos, id: \.id) { repo in
                        let open = repo.id == model.state.selected
                        Button { Task { await model.select(repo.id) } } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(repo.name)
                                    .font(.system(size: 16))
                                    .foregroundColor(open ? Ink.text : Ink.dim)
                                Text("\(repo.url)  \(repo.branch)")
                                    .font(.clarityMono).foregroundColor(Ink.dim)
                                    .lineLimit(1)
                            }
                        }
                        .listRowBackground(open ? Ink.surface : Ink.bg)
                        // Removing is behind a press-and-hold rather than a
                        // control on every row: it is rare, it is destructive,
                        // and a row whose purpose is being tapped should not
                        // carry a second thing to tap by mistake.
                        .contextMenu {
                            Button("Remove", role: .destructive) {
                                Task { await model.removeRepo(repo.id) }
                            }
                        }
                    }
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
            }
            }
            }
        }
        .background(Ink.surface)
    }
}
