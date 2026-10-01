import SwiftUI

/// One repository's commits, in the same three-section shape the TUI shows:
/// what has landed, what is green, and what has shipped.
///
/// Every grouping and status decision arrived settled in the view — this file
/// only decides how wide things are, which is the half of the job the proto
/// boundary leaves to the platform.
struct RepoView: View {
    let repoID: String
    @ObservedObject var model: ClarityModel

    @State private var tab = 0

    private var flows: [Clarity_V1_Flow] { model.state.view?.flows ?? [] }
    private var flow: Clarity_V1_Flow? {
        flows.isEmpty ? nil : flows[min(tab, flows.count - 1)]
    }

    var body: some View {
        VStack(spacing: 0) {
            TopBar(title: model.state.repo?.name ?? "", onBack: { model.back() }) {
                Button(model.state.syncing ? "fetching…" : "refresh") {
                    Task { await model.refresh() }
                }
                .font(.system(size: 13))
                .foregroundColor(Ink.blue)
                .disabled(model.state.syncing)
            }
            ErrorBar(error: model.state.error) { model.dismissError() }

            // A single-target repo has exactly one flow, and a tab bar over one
            // tab is just a wasted row.
            if flows.count > 1 {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 20) {
                        ForEach(Array(flows.enumerated()), id: \.offset) { index, f in
                            Button { tab = index } label: {
                                HStack(spacing: 6) {
                                    StatusGlyph(status: f.deploy)
                                    Text(f.undeclared ? "\(f.name) ?" : f.name)
                                        .font(.system(size: 13))
                                        .foregroundColor(index == tab ? Ink.text : Ink.dim)
                                }
                            }
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.bottom, 8)
                }
            }

            if model.state.view == nil {
                Spacer()
                Text(model.state.syncing ? "Fetching…" : "Nothing fetched yet.")
                    .font(.system(size: 14)).foregroundColor(Ink.dim)
                Spacer()
            } else {
                List {
                    ForEach(Array((flow?.groups ?? []).enumerated()), id: \.offset) { _, group in
                        Section {
                            ForEach(group.commits, id: \.sha) { commit in
                                CommitRow(commit: commit).listRowBackground(Ink.bg)
                            }
                        } header: {
                            GroupHeader(group: group)
                        }
                    }
                    if model.state.view?.truncated == true {
                        Text("Showing the most recent \(model.state.view?.limit ?? 0) commits.")
                            .font(.system(size: 12)).foregroundColor(Ink.dim)
                            .listRowBackground(Ink.bg)
                    }
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
            }
        }
    }
}

private struct GroupHeader: View {
    let group: Clarity_V1_Group

    var body: some View {
        HStack(spacing: 8) {
            StatusGlyph(status: group.status)
            Text(group.label).font(.system(size: 13)).foregroundColor(Ink.text)
            Spacer()
            if !group.deployedAgo.isEmpty {
                Text(group.deployedAgo).font(.system(size: 12)).foregroundColor(Ink.dim)
            }
        }
    }
}

private struct CommitRow: View {
    let commit: Clarity_V1_Commit

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            HStack(spacing: 4) {
                StatusGlyph(status: commit.ci)
                StatusGlyph(status: commit.deploy)
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(commit.subject)
                    .font(.system(size: 14)).foregroundColor(Ink.text)
                    .lineLimit(2)
                HStack(spacing: 8) {
                    Text(commit.shortSha).font(.system(size: 11, design: .monospaced))
                    Text(commit.author).font(.system(size: 11)).lineLimit(1)
                    Text(commit.age).font(.system(size: 11))
                }
                .foregroundColor(Ink.dim)
            }
            Spacer(minLength: 8)
            if commit.hasLeadTime_p {
                // A live lead time is still counting, and saying so is the
                // difference between "this took 3m" and "this has taken 3m so
                // far".
                Text(commit.leadTimeLive ? "\(commit.leadTime)…" : commit.leadTime)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(commit.leadTimeLive ? Ink.yellow : Ink.dim)
            }
        }
        .padding(.vertical, 4)
    }
}
