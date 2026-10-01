import SwiftUI

/// The menu of tracked repositories.
struct ReposView: View {
    @ObservedObject var model: ClarityModel

    var body: some View {
        VStack(spacing: 0) {
            TopBar(title: "clarity") {
                HStack(spacing: 16) {
                    Button("key") { Task { await model.showKey() } }
                        .font(.system(size: 13)).foregroundColor(Ink.dim)
                    Button("add") { model.showAddRepo() }
                        .font(.system(size: 13)).foregroundColor(Ink.blue)
                }
            }
            ErrorBar(error: model.state.error) { model.dismissError() }

            if model.state.repos.isEmpty {
                Spacer()
                VStack(alignment: .leading, spacing: 8) {
                    Text("No repositories yet.")
                        .font(.system(size: 15)).foregroundColor(Ink.text)
                    Text("Add one by pasting the URL you would clone. If it is an ssh URL, "
                        + "put this device's key on the host first.")
                        .font(.system(size: 13)).foregroundColor(Ink.dim)
                }
                .padding(24)
                Spacer()
            } else {
                List {
                    ForEach(model.state.repos, id: \.id) { repo in
                        Button { Task { await model.openRepo(repo.id) } } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(repo.name).font(.system(size: 16)).foregroundColor(Ink.text)
                                Text("\(repo.url)  \(repo.branch)")
                                    .font(.clarityMono).foregroundColor(Ink.dim)
                                    .lineLimit(1)
                            }
                        }
                        .listRowBackground(Ink.bg)
                        .swipeActions {
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
