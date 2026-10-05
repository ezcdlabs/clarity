import SwiftUI

/// The page margin, and the column everything that is not a mark starts at.
let pageMargin: CGFloat = 20

/**
 The whole app.

 One repository, with everything else presented over it. On iOS "over" means
 sheets rather than full-screen replacements: a sheet can be pulled down, which
 is the gesture an iOS user reaches for before they look for a close button, and
 it keeps the thing you were reading visible behind it.
 */
struct RootView: View {
    @ObservedObject var model: ClarityModel

    var body: some View {
        Group {
            if model.state.isEmpty {
                EmptyStateView(model: model)
            } else {
                RepoView(model: model)
            }
        }
        .background(Ink.bg)
        // One sheet, not one per overlay.
        //
        // Three sheets with a boolean each cannot swap: going from the list
        // straight to the connect flow dismisses one and presents another in a
        // single frame, and SwiftUI reconciles one presentation per turn — the
        // second is simply lost. `sheet(item:)` is the API for this. A change
        // of identity is a swap it knows how to animate.
        .sheet(item: presented) { overlay in
            switch overlay {
            case .connect:
                ConnectView(model: model)
            case .key:
                KeyView(model: model)
            case .switcher:
                SwitcherView(model: model)
                    .presentationDetents([.medium, .large])
                    .presentationDragIndicator(.visible)
            }
        }
    }

    /// What is over the repository, if anything. Dismissing it — including by
    /// dragging it down, which no code of ours runs — clears the overlay, so
    /// the model never believes a sheet is up that is not.
    private var presented: Binding<Overlay?> {
        Binding(
            get: { model.state.overlay },
            set: { shown in if shown == nil { model.closeOverlay() } },
        )
    }
}

/**
 The error, if there is one, under whatever is on screen.

 Deliberately not an alert: the messages come from the core and from git itself,
 they are often long, and they usually describe why the thing behind them is
 stale rather than why it is absent. Covering the data up to explain that it is
 old would be the wrong trade.
 */
struct ErrorBar: View {
    let error: String?
    let onDismiss: () -> Void

    var body: some View {
        if let error {
            HStack(alignment: .top, spacing: 8) {
                Text(error)
                    .font(Type.supporting)
                    .foregroundColor(Ink.red)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer()
                Button(action: onDismiss) {
                    Image(systemName: "xmark").font(.system(size: 13))
                }
                .foregroundColor(Ink.dim)
                .accessibilityLabel("Dismiss")
            }
            .padding(.horizontal, pageMargin)
            .padding(.vertical, 10)
            .background(Ink.errorBg)
        }
    }
}

/// The one prominent button a screen is allowed.
///
/// `.borderedProminent` at `.large` rather than Android's fully-rounded pill:
/// the pill is Material's shape, and a rounded rectangle is what every other
/// button on this phone looks like.
struct PrimaryButton: View {
    let title: String
    var systemImage: String?
    var enabled = true
    var working = false
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            HStack(spacing: 8) {
                if working {
                    ProgressView().controlSize(.small).tint(Ink.dim)
                } else if let systemImage {
                    Image(systemName: systemImage)
                }
                Text(title).font(Type.button)
            }
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(.borderedProminent)
        .controlSize(.large)
        .tint(Ink.blue)
        .foregroundColor(Ink.bg)
        .disabled(!enabled || working)
    }
}
