import SwiftUI

/// The glyph and colour for a status.
///
/// The proto deliberately sends the status and not a glyph, because the TUI's
/// ✓/✗ were chosen to survive a greyscale terminal. They survive here too, and
/// keeping them means someone who uses both reads the same marks — but colour is
/// never the only signal, which is the part that actually mattered.
///
/// `stale` mutes a result a newer commit has already superseded: a build that
/// broke three commits ago and has since gone green is history, not an alarm.
struct StatusGlyph: View {
    let status: Clarity_V1_Status
    var stale = false
    var size: CGFloat = 13

    var body: some View {
        Text(mark)
            .font(.system(size: size, design: .monospaced))
            .foregroundColor(colour)
    }

    private var mark: String {
        switch status {
        case .passed: return "✓"
        case .failed: return "✗"
        case .started: return "⋯"
        case .skipped: return "–"
        // Nothing reported is not a state to draw attention to: a commit nobody
        // has run anything against yet is normal, not pending.
        default: return "·"
        }
    }

    private var colour: Color {
        switch status {
        case .passed: return stale ? Ink.dim : Ink.green
        case .failed: return stale ? Ink.dim : Ink.red
        case .started: return Ink.yellow
        case .skipped: return Ink.dim
        default: return Ink.line
        }
    }
}
