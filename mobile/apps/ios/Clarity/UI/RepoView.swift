import SwiftUI

/// One repository, in the same three-band shape the TUI draws: what has landed,
/// what is green, and what has shipped.
///
/// Every grouping and status decision arrived settled in the view. What this
/// file decides is how wide things are and which colour they take, which is the
/// half of the job the proto boundary leaves to the platform.
struct RepoView: View {
    @ObservedObject var model: ClarityModel
    let onOpenList: () -> Void

    @State private var tab = 0

    private var flows: [Clarity_V1_Flow] { model.state.view?.flows ?? [] }
    private var flow: Clarity_V1_Flow? {
        flows.isEmpty ? nil : flows[min(tab, flows.count - 1)]
    }

    var body: some View {
        VStack(spacing: 0) {
            TopBar(
                title: model.state.repo?.name ?? "clarity",
                // The name goes red when something is broken, as it does in the
                // terminal: the one place the repository is always named is the
                // one place a verdict is always visible.
                titleColor: broken ? Ink.red : Ink.text,
                leading: {
                    // The list is a swipe away; this is for anyone who does not
                    // find a gesture that has no affordance.
                    Button(action: onOpenList) {
                        Text("≡").font(.system(size: 20, design: .monospaced)).foregroundColor(Ink.dim)
                    }
                },
                trailing: {
                    Button(model.state.syncing ? "fetching…" : "refresh") {
                        Task { await model.refresh() }
                    }
                    .font(.system(size: 13))
                    .foregroundColor(Ink.blue)
                    .disabled(model.state.syncing)
                }
            )
            if let view = model.state.view {
                header(view)
            }
            if model.state.syncing {
                ProgressView().progressViewStyle(.linear).tint(Ink.blue)
            }
            ErrorBar(error: model.state.error) { model.dismissError() }

            if model.state.view == nil {
                Spacer()
                Text(emptyMessage).font(.system(size: 14)).foregroundColor(Ink.dim)
                Spacer()
            } else {
                List {
                    ForEach(Array((flow?.sections ?? []).enumerated()), id: \.offset) { _, section in
                        Section {
                            if section.commits.isEmpty && section.batches.isEmpty {
                                Text(emptyLine(section.kind))
                                    .font(.system(size: 13)).foregroundColor(Ink.line)
                                    .listRowBackground(Ink.bg)
                            }
                            ForEach(section.commits, id: \.sha) { commit in
                                CommitRow(commit: commit, model: model).listRowBackground(Ink.bg)
                            }
                            ForEach(Array(section.batches.enumerated()), id: \.offset) { _, batch in
                                BatchHeader(batch: batch, model: model).listRowBackground(Ink.bg)
                                ForEach(batch.commits, id: \.sha) { commit in
                                    CommitRow(commit: commit, model: model).listRowBackground(Ink.bg)
                                }
                            }
                        } header: {
                            Text(section.label)
                                .font(.system(size: 13, weight: .bold))
                                .foregroundColor(accent(section.kind))
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
        .background(Ink.bg)
    }

    /// Whether anything in this repository is currently failing.
    private var broken: Bool {
        guard let view = model.state.view else { return false }
        return view.ci == .failed || view.deploy == .failed
    }

    /// The header answers two questions with different scopes: CI is repo-wide,
    /// deploys are per-flow. Naming the group is what says so — `deploy:` labels
    /// the strip, so the flows read as sub-items of deploy rather than as peers
    /// of `ci`.
    ///
    /// One flow renders flat, with no bar, because a lone raised tab looks like
    /// a control and is not one.
    @ViewBuilder
    private func header(_ view: Clarity_V1_View) -> some View {
        if view.flows.count <= 1 {
            HStack(spacing: 16) {
                badge("ci:", view.ci)
                badge("deploy:", view.flows.first?.deploy ?? view.deploy)
                Spacer()
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 6)
        } else {
            // The selected flow is cut out of the chrome rather than raised
            // above it: the bar is painted one step off the background and the
            // selected tab in the background itself, so it is the only thing on
            // the row sharing the body's colour. That reads as a tab continuous
            // with what it controls.
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 0) {
                    badge("ci:", view.ci).padding(.leading, 16).padding(.trailing, 16)
                    Text("deploy:").font(.system(size: 13)).foregroundColor(Ink.dim)
                    ForEach(Array(view.flows.enumerated()), id: \.offset) { index, f in
                        Button { tab = index } label: {
                            HStack(spacing: 6) {
                                // Undeclared: seen in the events but absent
                                // from .ezcd.json.
                                Text(f.undeclared ? "\(f.name) ?" : f.name)
                                    .font(.system(size: 13))
                                    .foregroundColor(index == tab ? Ink.text : Ink.dim)
                                // Status sits after the name, matching `ci: ✓`.
                                StatusGlyph(status: f.deploy, prominent: true)
                            }
                            .padding(.horizontal, 10)
                            .padding(.vertical, 8)
                            .background(index == tab ? Ink.bg : Color.clear)
                        }
                    }
                }
            }
            .background(Ink.surface)
        }
    }

    private func badge(_ label: String, _ status: Clarity_V1_Status) -> some View {
        HStack(spacing: 6) {
            // Lowercase, as the header has always shipped.
            Text(label).font(.system(size: 13)).foregroundColor(Ink.dim)
            StatusGlyph(status: status, prominent: true)
        }
    }

    private var emptyMessage: String {
        if model.state.syncing { return "Fetching…" }
        if model.state.repos.isEmpty { return "Add a repository to begin." }
        return "Nothing fetched yet."
    }
}

/// What an empty band says, so the frame still reads as an answer.
private func emptyLine(_ kind: Clarity_V1_SectionKind) -> String {
    switch kind {
    case .head: return "nothing waiting on CI"
    case .ciPassed: return "nothing waiting to deploy"
    case .deployed: return "nothing deployed yet"
    default: return ""
    }
}

/// The lifecycle accents the TUI uses: HEAD neutral, CI Passed yellow, Deployed
/// blue. The colour marks the band, not a status — a yellow CI Passed header
/// does not mean anything is wrong.
private func accent(_ kind: Clarity_V1_SectionKind) -> Color {
    switch kind {
    case .ciPassed: return Ink.yellow
    case .deployed: return Ink.blue
    default: return Ink.text
    }
}

private struct BatchHeader: View {
    let batch: Clarity_V1_Batch
    @ObservedObject var model: ClarityModel

    var body: some View {
        HStack {
            Text(batch.label)
                .font(.system(size: 12, weight: batch.live ? .bold : .regular))
                .foregroundColor(colour)
            Spacer()
            let ago = ticking(model, batch.deployedUnixSeconds, batch.deployedAgo) { "\($0) ago" }
            if !ago.isEmpty {
                Text(ago).font(.system(size: 12)).foregroundColor(Ink.dim)
            }
        }
    }

    private var colour: Color {
        switch batch.status {
        case .failed: return Ink.red
        case .started: return Ink.dim
        default: return Ink.blue
        }
    }
}

/// One commit, laid out the way the terminal lays one out: a status icon, the
/// author, the subject, and a right-aligned lead time. One line.
///
/// No sha — the terminal does not print one, and nothing on a phone can be
/// copied out of a list row anyway. No age either: the only timer on a row is
/// the lead time, and a second one beside it invites the reader to work out
/// which is which.
private struct CommitRow: View {
    let commit: Clarity_V1_Commit
    @ObservedObject var model: ClarityModel

    /// A fixed column, so subjects start at the same place down the list rather
    /// than stepping in and out with the length of a name.
    private let authorColumn: CGFloat = 88

    var body: some View {
        HStack(spacing: 10) {
            // One mark, not two. Whether this commit shipped is said by the band
            // and the batch it sits in, which is why the terminal has only ever
            // drawn the CI result here.
            StatusGlyph(status: commit.ci, stale: commit.ciStale)

            Text(commit.author)
                .font(.system(size: 13))
                .foregroundColor(Ink.dim)
                .lineLimit(1)
                .truncationMode(.tail)
                .frame(width: authorColumn, alignment: .leading)

            Text(commit.subject)
                .font(.system(size: 13))
                .foregroundColor(Ink.text)
                .lineLimit(1)
                .truncationMode(.tail)
                .frame(maxWidth: .infinity, alignment: .leading)

            if commit.hasLeadTime_p {
                // Grey while it runs, blue once the deploy that stopped the
                // clock landed — so a lead time blooms blue exactly when it
                // freezes, matching the Deployed band it came to rest in.
                Text(
                    commit.leadTimeLive
                        ? ticking(model, commit.leadTimeAnchorUnixSeconds, commit.leadTime) { $0 }
                        : commit.leadTime
                )
                .font(.system(size: 12, design: .monospaced))
                .foregroundColor(commit.leadTimeLive ? Ink.dim : Ink.blue)
                .lineLimit(1)
            }
        }
        .padding(.vertical, 3)
    }
}

/// A duration that keeps counting between fetches.
///
/// The view arrives with every duration preformatted, which is right for the
/// instant it was built and wrong a second later. Given an anchor, this
/// recomputes against the model's clock; without one — or before the clock has
/// started — it falls back to what the view said, which is never worse than
/// what the last fetch showed.
private func ticking(
    _ model: ClarityModel,
    _ anchorUnixSeconds: Int64,
    _ fallback: String,
    _ format: (String) -> String
) -> String {
    guard anchorUnixSeconds > 0, model.state.nowSeconds > 0 else { return fallback }
    return format(model.elapsed(model.state.nowSeconds - anchorUnixSeconds))
}
