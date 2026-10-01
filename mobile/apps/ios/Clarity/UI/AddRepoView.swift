import SwiftUI

/// Paste a clone URL.
///
/// Nothing is validated here. The registry already decides what a git remote
/// looks like and writes the message explaining a rejection; a second opinion in
/// Swift could only disagree with it.
struct AddRepoView: View {
    @ObservedObject var model: ClarityModel

    @State private var url = ""
    @State private var branch = ""

    var body: some View {
        VStack(spacing: 0) {
            TopBar(title: "Add a repository", onBack: { model.back() }) { EmptyView() }
            ErrorBar(error: model.state.error) { model.dismissError() }

            VStack(alignment: .leading, spacing: 12) {
                TextField("git@github.com:you/thing.git", text: $url)
                    .font(.clarityMono)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .padding(12)
                    .background(Ink.surface)

                TextField("main", text: $branch)
                    .font(.clarityMono)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .padding(12)
                    .background(Ink.surface)

                Text("Leave the branch blank for main. An ssh URL uses this device's key — "
                    + "add it on the host first, under “key”.")
                    .font(.system(size: 12)).foregroundColor(Ink.dim)

                Button {
                    Task { await model.addRepo(url: url, branch: branch) }
                } label: {
                    Text(model.state.busy ? "Adding…" : "Add")
                        .foregroundColor(Ink.blue)
                }
                .disabled(model.state.busy || url.isEmpty)
                .padding(.top, 8)
            }
            .foregroundColor(Ink.text)
            .padding(16)

            Spacer()
        }
    }
}
