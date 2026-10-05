import SwiftUI

/**
 A status, as a mark.

 SF Symbols rather than the ✓ and ✗ characters the terminal uses, and rather
 than the Material Symbols Android draws. The shapes still carry the meaning —
 that was the point of the terminal's choice and it survives both moves — but
 the marks beside them in the navigation bar are SF Symbols, and one icon set
 per screen is the whole of looking native.

 `prominent` is where colour goes. The header is the summary and earns it; row
 marks forgo it and spend red only on a failure still breaking something. A
 screen of green ticks spends the one colour meaning "look at this" on the state
 needing no attention at all.

 `stale` mutes a result a newer commit has superseded: a build that broke three
 commits ago and has since gone green is history, not an alarm.
 */
/// No status at all, as a value rather than as `.none`.
///
/// `.none` beside an optional is the one place Swift's two meanings for that
/// name meet, and the error it produces mentions neither.
let unreported = Clarity_V1_Status()

struct StatusGlyph: View {
    let status: Clarity_V1_Status
    var prominent = false
    var stale = false
    var size: CGFloat = 16

    var body: some View {
        Group {
            switch status {
            case .passed:
                Image(systemName: "checkmark").accessibilityLabel("passed")
            case .failed:
                Image(systemName: "xmark").accessibilityLabel("failed")
            case .started:
                Image(systemName: "ellipsis").accessibilityLabel("in progress")
            default:
                // Nothing reported. A small square rather than a shrunken
                // symbol: it has to read as an empty slot, not as a mark too
                // faint to make out, and at this size any symbol would.
                Rectangle()
                    .frame(width: 4, height: 4)
                    .accessibilityLabel("none")
            }
        }
        .font(.system(size: size, weight: .medium))
        .foregroundColor(colour)
        .frame(width: size, height: size)
    }

    private var colour: Color {
        if prominent {
            switch status {
            case .passed: return Ink.green
            case .failed: return Ink.red
            default: return Ink.dim
            }
        }
        if status == .failed && !stale { return Ink.red }
        return Ink.dim
    }
}
