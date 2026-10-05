import SwiftUI
import UIKit

/**
 One repository, in the same three-band shape the TUI draws: what has landed,
 what is green, and what has shipped.

 Every grouping and status decision arrived settled in the view. What this file
 decides is how wide things are and which colour they take — the half of the job
 the proto boundary leaves to the platform. Android decides the same things from
 the same view, and the two answers differ wherever the platforms differ: a
 navigation bar instead of a 64dp app bar, a `Menu` instead of a `DropdownMenu`,
 `.refreshable` instead of a `PullToRefreshBox`.
 */
struct RepoView: View {
    @ObservedObject var model: ClarityModel

    /// Which deploy target is on show. Keyed on the repository, so switching
    /// repositories does not land you on the fourth flow of one that has two.
    @State private var tab = 0

    @State private var renaming: Clarity_V1_RepoSummary?
    @State private var changingBranch: Clarity_V1_RepoSummary?
    @State private var removing: Clarity_V1_RepoSummary?

    private var state: AppState { model.state }
    private var flows: [Clarity_V1_Flow] { state.view?.flows ?? [] }
    private var flow: Clarity_V1_Flow? {
        flows.isEmpty ? nil : flows[min(tab, flows.count - 1)]
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                header
                ErrorBar(error: state.error) { model.dismissError() }
                feed
            }
            .background(Ink.bg)
            .navigationBarTitleDisplayMode(.inline)
            .toolbarBackground(Ink.surface, for: .navigationBar)
            .toolbarBackground(.visible, for: .navigationBar)
            .toolbar {
                ToolbarItem(placement: .principal) { titleButton }
                ToolbarItem(placement: .navigationBarTrailing) { menu }
            }
        }
        .onChange(of: state.selected) { _ in tab = 0 }
        .sheet(item: $renaming) { RenameSheet(repo: $0, model: model) }
        .sheet(item: $changingBranch) { BranchSheet(repo: $0, model: model) }
        .alert(
            "Remove \(removing?.title ?? "")?",
            isPresented: presenting($removing),
            presenting: removing
        ) { repo in
            Button("Remove", role: .destructive) { Task { await model.removeRepo(repo.id) } }
            Button("Cancel", role: .cancel) {}
        } message: { _ in
            Text(removalWarning)
        }
    }

    // MARK: - the bar

    /// The title is the switcher.
    ///
    /// A navigation title with a chevron, which on iOS is the established way to
    /// say "this screen is about one of several things" — the same control
    /// Safari puts over a tab group. Android gets a two-line title because its
    /// app bar is 64dp and ours is 44pt: the branch and host move to the row
    /// below, where they also have room to sit beside the fetch spinner.
    private var titleButton: some View {
        Button { model.showSwitcher() } label: {
            HStack(spacing: 2) {
                if let repo = state.repo, !repo.titlePrefix.isEmpty {
                    Text(repo.titlePrefix)
                        .font(Type.appBarTitle.weight(.regular))
                        .foregroundColor(Ink.dim)
                        .lineLimit(1)
                        .truncationMode(.head)
                }
                Text(state.repo?.title ?? state.view?.repoName ?? "clarity")
                    .font(Type.appBarTitle)
                    // Red when something is broken, as the terminal does with
                    // the repository name.
                    .foregroundColor(broken ? Ink.red : Ink.text)
                    .lineLimit(1)
                Image(systemName: "chevron.down")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(Ink.dim)
                    .padding(.leading, 2)
            }
        }
        .accessibilityLabel("Switch repository")
    }

    /// The per-repository menu.
    ///
    /// No refresh button: pulling the feed down is how a list is refreshed on
    /// this platform, and the menu's first item does it too — saying how old the
    /// view is, which is the question that prompts a refresh in the first place.
    private var menu: some View {
        Menu {
            Button { Task { await model.refresh() } } label: {
                // A menu row cannot hold a trailing note the way Android's can,
                // so the age goes in the label. It is the reason the item is
                // being read, not a detail beside it.
                Label(age.isEmpty ? "Refresh now" : "Refresh now · \(age)", systemImage: "arrow.clockwise")
            }
            if let repo = state.repo {
                Button { renaming = repo } label: { Label("Rename…", systemImage: "pencil") }
                Button { changingBranch = repo } label: {
                    Label("Change branch…", systemImage: "arrow.triangle.branch")
                }
                Button {
                    UIPasteboard.general.string = repo.url
                } label: {
                    Label("Copy clone address", systemImage: "doc.on.doc")
                }
                Section {
                    Button(role: .destructive) { removing = repo } label: {
                        Label("Remove repository", systemImage: "trash")
                    }
                }
            }
        } label: {
            Image(systemName: "ellipsis.circle")
        }
        .tint(Ink.blue)
    }

    /// How old what you are looking at is, ticking.
    private var age: String {
        guard let anchor = state.view?.generatedUnixSeconds, anchor > 0, state.nowSeconds > 0 else {
            return ""
        }
        return "\(model.elapsed(state.nowSeconds - anchor)) ago"
    }

    private var broken: Bool {
        guard let view = state.view else { return false }
        return view.ci == .failed || view.deploy == .failed
    }

    // MARK: - the summary

    /// Everything between the bar and the feed, on the chrome's tone.
    ///
    /// Two tones rather than Android's rounded sheet: the sheet that lifts and
    /// slides under the bar is Material's shape, and on iOS a change of ground
    /// is how a reading surface is separated from the chrome above it.
    private var header: some View {
        VStack(alignment: .leading, spacing: 4) {
            subtitleRow
            summaryRow
            if flows.count > 1 { flowChips }
        }
        .padding(.top, 4)
        .padding(.bottom, flows.count > 1 ? 8 : 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Ink.surface)
    }

    private var subtitleRow: some View {
        HStack(spacing: 8) {
            if let repo = state.repo {
                Text(repo.subtitle)
                    .font(Type.mono)
                    .foregroundColor(Ink.dim)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            // Fetching, said as quietly as it can be said: a spinner in space
            // that is already there, rather than a bar that pushes the feed
            // down every time a refresh runs.
            if state.syncing {
                ProgressView().controlSize(.mini).tint(Ink.dim)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, pageMargin)
    }

    /// CI for the repository, deploy for the flow.
    ///
    /// Naming the group is what says the two have different scopes: `deploy:`
    /// labels the chips below it, so they read as sub-items of deploy rather
    /// than as peers of `ci`.
    private var summaryRow: some View {
        HStack(spacing: 6) {
            Text("ci:").font(Type.monoMeta).foregroundColor(Ink.dim)
            StatusGlyph(status: state.view?.ci ?? unreported, prominent: true)
            Text("deploy:").font(Type.monoMeta).foregroundColor(Ink.dim).padding(.leading, 12)
            if flows.count <= 1 {
                StatusGlyph(status: flows.first?.deploy ?? state.view?.deploy ?? unreported, prominent: true)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, pageMargin)
    }

    /**
     One chip per deploy target.

     Not a segmented `Picker`, which was the first thing tried: a segmented
     control takes a label or an image but not both, so the per-flow result
     would have had to come out of the control and go somewhere else — and then
     nothing would say which flow the failing mark belonged to. A scrolling row
     of capsules is the other control iOS uses for this (News, Music, Photos),
     and it can carry the mark beside the name, which is the whole point.
     */
    private var flowChips: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(Array(flows.enumerated()), id: \.offset) { i, f in
                    let selected = i == min(tab, flows.count - 1)
                    Button { tab = i } label: {
                        HStack(spacing: 6) {
                            // A flow nothing has declared a target for, marked
                            // as the terminal marks it.
                            Text(f.undeclared ? "\(f.name) ?" : f.name)
                                .font(Type.monoMeta)
                                .foregroundColor(selected ? Ink.text : Ink.dim)
                            StatusGlyph(status: f.deploy, prominent: true, size: 14)
                        }
                        .padding(.horizontal, 12)
                        .padding(.vertical, 7)
                        .background(
                            Capsule().fill(selected ? Ink.line : Color.clear)
                        )
                        .overlay(
                            Capsule().strokeBorder(selected ? Color.clear : Ink.line, lineWidth: 1)
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, pageMargin)
            .padding(.top, 2)
        }
    }

    // MARK: - the feed

    @ViewBuilder
    private var feed: some View {
        if let view = state.view, let flow {
            List {
                ForEach(Array(flow.sections.enumerated()), id: \.offset) { _, section in
                    SectionHeaderView(section: section).feedRow()

                    ForEach(section.commits, id: \.sha) { commit in
                        CommitRow(commit: commit, model: model).feedRow()
                    }

                    ForEach(Array(section.batches.enumerated()), id: \.offset) { b, batch in
                        if !batch.weekLabel.isEmpty {
                            WeekDivider(label: batch.weekLabel).feedRow()
                        }
                        BatchHeader(
                            batch: batch,
                            model: model,
                            // Something already separated this one: the section
                            // rule it opens, or the week divider naming it.
                            // Spending the gap twice would push the subheader
                            // away from the commits it describes.
                            tight: !batch.weekLabel.isEmpty || b == 0
                        ).feedRow()
                        ForEach(batch.commits, id: \.sha) { commit in
                            CommitRow(commit: commit, model: model).feedRow()
                        }
                    }
                }
                if view.truncated {
                    Text("Showing the most recent \(view.limit) commits.")
                        .font(Type.supporting)
                        .foregroundColor(Ink.dim)
                        .padding(pageMargin)
                        .feedRow()
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .background(Ink.bg)
            // The platform's own pull to refresh, rather than Android's
            // suppressed indicator: here the spinner only appears because
            // someone pulled for it, so it is an answer to a question that was
            // actually asked.
            .refreshable { await model.refresh() }
        } else {
            VStack {
                Spacer()
                Text(emptyMessage).font(Type.bodySmall).foregroundColor(Ink.dim)
                Spacer()
            }
            .frame(maxWidth: .infinity)
        }
    }

    private var emptyMessage: String {
        if state.syncing { return "Fetching…" }
        if state.repos.isEmpty { return "Add a repository to begin." }
        return "Nothing fetched yet."
    }
}

/**
 The feed's grid, from the handoff.

 Everything sits inside a 20pt page margin. Within a row the status mark gets a
 16pt column and everything else starts 26pt in, so authors, subjects and
 section labels share one left edge — the thing the terminal gets for free by
 counting characters.

 A batch label is the exception, deliberately: it aligns to the *glyph* column
 rather than the text, because it labels the marks below it rather than standing
 beside them. The terminal does the same.
 */
private let glyphColumn: CGFloat = 16
private let textInset: CGFloat = 26

/**
 The vertical rhythm, which is the only thing saying what belongs to what.

 The terminal separates a deploy from the one above it with a blank line and
 binds it to its own commits by adjacency. On a phone those two gaps have to be
 visibly different sizes or the subheader floats between the batch above and the
 batch below, attached to neither.
 */
private let commitGap: CGFloat = 14
private let batchGapAbove: CGFloat = 24
private let batchGapBelow: CGFloat = 6

extension View {
    /// A feed row, stripped of everything a `List` would otherwise add.
    ///
    /// The list is here for its laziness and its pull to refresh, not its
    /// decoration: the separators, insets and selection tint belong to a
    /// settings table, and this is a commit log whose own spacing is what says
    /// which lines go together.
    func feedRow() -> some View {
        listRowInsets(EdgeInsets())
            .listRowSeparator(.hidden)
            .listRowBackground(Ink.bg)
    }
}

/**
 A band header, with the rule that makes it the head of what follows.

 The lifecycle accents are the TUI's: HEAD neutral, CI Passed yellow, Deployed
 blue. The colour marks the band, not a status — a yellow CI Passed header does
 not mean anything is wrong.
 */
private struct SectionHeaderView: View {
    let section: Clarity_V1_Section

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .lastTextBaseline) {
                Text(section.label).font(Type.band).foregroundColor(accent)
                Spacer()
                // This week's throughput rides on the Deployed rule rather than
                // taking a row of its own, as it does in the terminal — this
                // week is the one a reader is asking about.
                if !section.summary.isEmpty {
                    Text(section.summary)
                        .font(Type.monoMeta)
                        .italic()
                        .foregroundColor(Ink.dim)
                }
            }
            .padding(.leading, pageMargin + textInset)
            .padding(.trailing, pageMargin)
            FeedRule()
            Spacer().frame(height: 10)
        }
        .padding(.top, 24)
    }

    private var accent: Color {
        switch section.kind {
        case .ciPassed: return Ink.yellow
        case .deployed: return Ink.blue
        default: return Ink.text
        }
    }
}

/// The hairline under a band label or a week, spanning the page margins.
///
/// What makes a label read as the head of what follows rather than as a stray
/// line of text — the job the terminal gives to a run of dashes. Drawn rather
/// than a `Divider`, which inherits a list's inset and would start in a
/// different place from the rule above it.
private struct FeedRule: View {
    var body: some View {
        Rectangle()
            .fill(Ink.line)
            .frame(height: 1)
            .padding(.horizontal, pageMargin)
            .padding(.vertical, 4)
    }
}

/**
 A week other than the current one, named above its first batch.

 The less-prominent sibling of the section rule: right-aligned, because it is
 peripheral context about the rows below rather than a heading for them. It gets
 the same rule, though — without one it floats between two batches, belonging to
 neither.
 */
private struct WeekDivider: View {
    let label: String

    var body: some View {
        VStack(alignment: .trailing, spacing: 0) {
            HStack {
                Spacer()
                Text(label).font(Type.monoMeta).italic().foregroundColor(Ink.dim)
            }
            .padding(.horizontal, pageMargin)
            FeedRule()
        }
        .padding(.top, 12)
    }
}

/// One deploy's subheader.
private struct BatchHeader: View {
    let batch: Clarity_V1_Batch
    @ObservedObject var model: ClarityModel
    let tight: Bool

    var body: some View {
        // One line, with the time inline, exactly as the terminal writes it:
        // "live on production · deployed 4m 43s ago". Pushing the time to the
        // right edge made it look like a column of its own and broke the
        // sentence.
        let ago = ticking(model, batch.deployedUnixSeconds, batch.deployedAgo) { "\($0) ago" }
        Text(ago.isEmpty ? batch.label : "\(batch.label) \(ago)")
            .font(.system(size: 12, weight: batch.live ? .bold : .regular))
            // The live batch is the present state rather than a past event, so
            // it is the one carrying weight.
            .italic(!batch.live)
            .foregroundColor(colour)
            .lineLimit(1)
            .frame(maxWidth: .infinity, alignment: .leading)
            // The glyph column, not the text column: it labels the marks below
            // it rather than standing beside them, which is the edge the
            // terminal uses too.
            .padding(.horizontal, pageMargin)
            .padding(.top, tight ? batchGapBelow : batchGapAbove)
            .padding(.bottom, batchGapBelow)
    }

    private var colour: Color {
        switch batch.status {
        case .failed: return Ink.red
        case .started: return Ink.dim
        default: return Ink.blue
        }
    }
}

/**
 One commit, on two lines.

 A phone is too narrow for the terminal's single row: at this width the subject
 is the first thing to be clipped, and it is the thing you are reading the list
 for. So the identity and the lead time share the top line, and the subject gets
 the full width below, indented to line up under the author.

 No sha — the terminal does not print one, and nothing on a phone can be copied
 out of a list row anyway. No age either: the only timer on a row is the lead
 time, and a second one beside it invites the reader to work out which is which.
 */
private struct CommitRow: View {
    let commit: Clarity_V1_Commit
    @ObservedObject var model: ClarityModel

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 0) {
                // One mark, not two. Whether this commit shipped is said by the
                // band and the batch it sits in, which is why the terminal has
                // only ever drawn the CI result here.
                StatusGlyph(status: commit.ci, stale: commit.ciStale)
                    .frame(width: glyphColumn, alignment: .leading)

                Text(commit.author)
                    .font(Type.meta)
                    .foregroundColor(Ink.dim)
                    .lineLimit(1)
                    .padding(.leading, 10)

                Spacer(minLength: 8)

                if commit.leadTimeKnown {
                    // Grey while it runs, blue once the deploy that stopped the
                    // clock landed — so a lead time blooms blue exactly when it
                    // freezes, matching the Deployed band it came to rest in.
                    Text(
                        commit.leadTimeLive
                            ? ticking(model, commit.leadTimeAnchorUnixSeconds, commit.leadTime) { $0 }
                            : commit.leadTime
                    )
                    .font(Type.monoMeta)
                    .foregroundColor(commit.leadTimeLive ? Ink.dim : Ink.blue)
                    .lineLimit(1)
                }
            }

            Text(commit.subject)
                .font(Type.subject)
                .foregroundColor(Ink.text)
                .lineLimit(2)
                // Nothing between the two lines. They are one commit, and the
                // only gap on this row that means anything is the one below it.
                .padding(.leading, textInset)
        }
        .padding(.horizontal, pageMargin)
        .padding(.bottom, commitGap)
    }
}

/**
 A duration that keeps counting between fetches.

 The view arrives with its durations preformatted, which is right for the
 instant it was built and wrong a second later. Given an anchor, this recomputes
 against the model's clock; without one — or before the clock has started — it
 falls back to what the view said, which is never worse than what the last fetch
 showed.

 `@MainActor` because it reads the model, which is: a free function is
 nonisolated unless it says otherwise, and `View.body` being isolated does not
 reach into what it calls.
 */
@MainActor
private func ticking(
    _ model: ClarityModel,
    _ anchorUnixSeconds: Int64,
    _ fallback: String,
    _ format: (String) -> String
) -> String {
    guard anchorUnixSeconds > 0, model.state.nowSeconds > 0 else { return fallback }
    return format(model.elapsed(model.state.nowSeconds - anchorUnixSeconds))
}
