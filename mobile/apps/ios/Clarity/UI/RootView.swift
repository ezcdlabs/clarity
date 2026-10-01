import SwiftUI

/// The whole app: one state value in, one screen out.
///
/// Routing is a `switch` over `Screen` rather than NavigationStack. There are
/// four destinations and no deep links, and a navigation path here would be a
/// second place for state to live.
struct RootView: View {
    @ObservedObject var model: ClarityModel

    var body: some View {
        ZStack {
            Ink.bg.ignoresSafeArea()
            switch model.state.screen {
            case .repos:
                ReposView(model: model)
            case .addRepo:
                AddRepoView(model: model)
            case .key:
                KeyView(model: model)
            case let .repo(id):
                RepoView(repoID: id, model: model)
            }
        }
        .preferredColorScheme(.dark)
    }
}

/// A title row with an optional back arrow and trailing content.
struct TopBar<Trailing: View>: View {
    let title: String
    var onBack: (() -> Void)?
    @ViewBuilder var trailing: Trailing

    var body: some View {
        HStack(spacing: 8) {
            if let onBack {
                Button(action: onBack) {
                    Text("‹").font(.system(size: 26)).foregroundColor(Ink.dim)
                }
            }
            Text(title).font(.system(size: 18)).foregroundColor(Ink.text)
            Spacer()
            trailing
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
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
