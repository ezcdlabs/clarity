import SwiftUI

/// The repository list and the repository, in that order.
private let pageRepos = 0
private let pageRepo = 1

/// The whole app: two pages side by side, the way Slack puts its channel list
/// beside the conversation.
///
/// Not a drawer. A drawer opens from an edge and sits over what it covers; this
/// swipes from anywhere and the two pages are peers, so going back to the list
/// is the same gesture as going back to the repository. The burger button does
/// the same thing for anyone who does not go looking for a gesture.
///
/// That the selection survives the trip is the model's doing, not the pager's —
/// moving to the list is not leaving the repository.
struct RootView: View {
    @ObservedObject var model: ClarityModel
    @State private var page = pageRepo

    var body: some View {
        ZStack {
            Ink.bg.ignoresSafeArea()

            switch model.state.overlay {
            case .addRepo:
                AddRepoView(model: model)
            case .key:
                KeyView(model: model)
            case .none:
                TabView(selection: $page) {
                    ReposPane(model: model).tag(pageRepos)
                    RepoView(model: model, onOpenList: { page = pageRepos }).tag(pageRepo)
                }
                .tabViewStyle(.page(indexDisplayMode: .never))
            }
        }
        // Picking a repository carries you to it. Here rather than in the tap
        // handler so it also happens when the selection changes for another
        // reason — the first launch, or the fallback after a removal.
        .onChange(of: model.state.selected) { _ in
            if model.state.selected != nil { withAnimation { page = pageRepo } }
        }
        .onAppear {
            // A fresh install has nothing to show on the repository page, so it
            // opens on the list, where the only useful thing to do is add one.
            if model.state.selected == nil { page = pageRepos }
        }
    }
}

/// A title row with a leading action and trailing content.
struct TopBar<Leading: View, Trailing: View>: View {
    let title: String
    var titleColor: Color = Ink.text
    @ViewBuilder var leading: Leading
    @ViewBuilder var trailing: Trailing

    var body: some View {
        HStack(spacing: 8) {
            leading
            Text(title).font(.system(size: 18)).foregroundColor(titleColor)
            Spacer()
            trailing
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
    }
}

extension TopBar where Leading == EmptyView {
    init(title: String, @ViewBuilder trailing: () -> Trailing) {
        self.init(title: title, titleColor: Ink.text, leading: { EmptyView() }, trailing: trailing)
    }
}

struct BackArrow: View {
    let action: () -> Void
    var body: some View {
        Button(action: action) {
            Text("‹").font(.system(size: 26)).foregroundColor(Ink.dim)
        }
    }
}

/// The error, if there is one, under whatever is on screen.
///
/// Deliberately not an alert: the messages come from the core and from git
/// itself, they are often long, and they usually describe why the thing behind
/// them is stale rather than why it is absent. Covering the data up to explain
/// that it is old would be the wrong trade.
struct ErrorBar: View {
    let error: String?
    let onDismiss: () -> Void

    var body: some View {
        if let error {
            HStack(alignment: .top) {
                Text(error)
                    .font(.system(size: 13))
                    .foregroundColor(Ink.red)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer()
                Button("dismiss", action: onDismiss)
                    .font(.system(size: 13))
                    .foregroundColor(Ink.dim)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 10)
            .background(Color(red: 0x2A / 255, green: 0x14 / 255, blue: 0x16 / 255))
        }
    }
}
