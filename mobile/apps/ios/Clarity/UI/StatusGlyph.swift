import SwiftUI

/// The glyph and colour for a status, following the terminal's two tables.
///
/// The proto sends the status and not a glyph, because the TUI's ✓/✗ were chosen
/// to survive a greyscale terminal. They survive here too — but colour is never
/// the only signal, which is the part that actually mattered.
///
/// **In a row** nothing is green. A tick is grey, and red is spent only on a
/// failure that is still breaking something: once a newer commit has gone green,
/// the older failure is history and goes grey with everything else. A screen of
/// green ticks spends the one colour that means "look at this" on the state that
/// needs no attention at all.
///
/// **In the header** the tick is green, because that row is the summary and its
/// whole job is answering "is the pipeline green?" at a glance. A started stage
/// shows as unresolved rather than as progress, the same as the terminal: the
/// header is a verdict, and "something is happening" is not one.
struct StatusGlyph: View {
    let status: Clarity_V1_Status
    var prominent = false
    var stale = false
    var size: CGFloat = 13

    var body: some View {
        let pair = prominent ? Self.summary(status) : Self.row(status, stale)
        Text(pair.0)
            .font(.system(size: size, design: .monospaced))
            .foregroundColor(pair.1)
    }

    /// The header table: green, red, or nothing resolved yet.
    private static func summary(_ status: Clarity_V1_Status) -> (String, Color) {
        switch status {
        case .passed: return ("✓", Ink.green)
        case .failed: return ("✗", Ink.red)
        default: return ("·", Ink.dim)
        }
    }

    /// The row table: shape carries the meaning, red carries the alarm.
    private static func row(_ status: Clarity_V1_Status, _ stale: Bool) -> (String, Color) {
        switch status {
        case .passed: return ("✓", Ink.dim)
        case .failed: return ("✗", stale ? Ink.dim : Ink.red)
        case .started: return ("⋯", Ink.dim)
        default: return ("·", Ink.dim)
        }
    }
}
