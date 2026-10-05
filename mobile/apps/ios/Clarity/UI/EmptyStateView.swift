import SwiftUI

/**
 First open, with nothing connected.

 It answers "what is this for?" before asking for anything, which is the only
 screen in the app with room to. The legend is the three bands the feed is made
 of, so the first repository that appears is already readable.
 */
struct EmptyStateView: View {
    @ObservedObject var model: ClarityModel

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 0) {
                Spacer()

                Text("NO REPOSITORIES YET")
                    .font(Type.overline)
                    .tracking(1.2)
                    .foregroundColor(Ink.dim)
                Text("Is main green?")
                    .font(Type.display)
                    .foregroundColor(Ink.text)
                    .padding(.top, 12)
                Text(
                    "Connect a repository to see what just landed, what passed CI "
                        + "and what's live in production. It's the same view as "
                        + "git clarity in your terminal."
                )
                .font(Type.body)
                .foregroundColor(Ink.dim)
                .padding(.top, 16)

                VStack(alignment: .leading, spacing: 14) {
                    legend(unreported, "HEAD", Ink.text, "what just landed")
                    legend(.passed, "CI Passed", Ink.yellow, "green, waiting to ship")
                    legend(.passed, "Deployed", Ink.blue, "live, with lead times")
                }
                .padding(.top, 28)

                Spacer()

                PrimaryButton(title: "Connect a repository", systemImage: "link.badge.plus") {
                    model.showConnect()
                }
                Text("Any git remote over SSH. Read-only.")
                    .font(Type.supporting)
                    .foregroundColor(Ink.dim)
                    .frame(maxWidth: .infinity)
                    .padding(.top, 12)
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Ink.bg)
            .navigationTitle("Git Clarity")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    Menu {
                        Button {
                            Task { await model.showKey() }
                        } label: {
                            Label("Device key", systemImage: "key")
                        }
                    } label: {
                        Image(systemName: "ellipsis.circle")
                    }
                    .tint(Ink.dim)
                }
            }
        }
    }

    /// One band, as it will look in the feed, with what it means beside it.
    private func legend(
        _ status: Clarity_V1_Status,
        _ band: String,
        _ colour: Color,
        _ meaning: String
    ) -> some View {
        HStack(spacing: 10) {
            StatusGlyph(status: status, size: 16)
            Text(band)
                .font(Type.supporting.weight(.semibold))
                .foregroundColor(colour)
                .frame(width: 76, alignment: .leading)
            Text(meaning).font(Type.supporting).foregroundColor(Ink.dim)
        }
    }
}
