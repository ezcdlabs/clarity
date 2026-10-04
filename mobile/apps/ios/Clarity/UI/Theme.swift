import SwiftUI
import UIKit

/// The TUI's palette, read on a screen instead of a terminal — and the brand's,
/// where a screen can hold it.
///
/// The terminal asks for ANSI 1/2/3/8/12 and lets the emulator decide what those
/// look like. A phone has no such table, so this is it: the same roles, answered
/// twice, with the mark's own colours where they carry.
enum Ink {
    /// Two tones: a chrome the bars sit on, and a darker sheet the commits are
    /// read on. A reading surface should be the extreme of the pair — every role
    /// gains contrast against it — and the step between them is tonal rather
    /// than a boundary, which is what makes them read as one ground with a
    /// sheet laid on it.
    ///
    /// Both are neutral. The brand navy was tried here and it was too much: a
    /// tint across a whole screen is not a brand, it is a cast, and it fought
    /// the one colour on the screen that is supposed to mean something.
    private static let dark = Shades(
        bg: hex(0x0E0E0E),
        surface: hex(0x1A1A1A),
        line: hex(0x2B2B2B),
        text: hex(0xE6E6E6),
        dim: hex(0x8C8C8C),
        red: hex(0xE06C75),
        green: hex(0x98C379),
        yellow: hex(0xF7C421), // the mark's yellow
        blue: hex(0x6AA2FF),   // the mark's blue
        errorBg: hex(0x2A1416)
    )

    /// Light cannot take the brand's colours at their own values: the mark's
    /// yellow on white is barely a colour and its blue is a highlight rather
    /// than something legible. The navy does carry, as the ink — a tint you
    /// read rather than one you sit in, which is the difference that made it
    /// work here and not as a ground.
    private static let light = Shades(
        bg: hex(0xFCFCFC),
        surface: hex(0xEDEDED),
        line: hex(0xDCDCDC),
        text: hex(0x061732),   // the mark's navy, as off-black
        dim: hex(0x6B6B6B),
        red: hex(0xC0392B),
        green: hex(0x2E7D32),
        yellow: hex(0x8A6D00),
        blue: hex(0x1565C0),
        errorBg: hex(0xFBE9E9)
    )

    struct Shades {
        let bg, surface, line, text, dim, red, green, yellow, blue, errorBg: Color
    }

    /// Resolved per trait collection, so the app follows the system the way the
    /// terminal follows its theme.
    static var bg: Color { dynamic(\.bg) }
    static var surface: Color { dynamic(\.surface) }
    static var line: Color { dynamic(\.line) }
    static var text: Color { dynamic(\.text) }
    static var dim: Color { dynamic(\.dim) }
    static var red: Color { dynamic(\.red) }
    static var green: Color { dynamic(\.green) }
    static var yellow: Color { dynamic(\.yellow) }
    static var blue: Color { dynamic(\.blue) }
    static var errorBg: Color { dynamic(\.errorBg) }

    private static func dynamic(_ role: KeyPath<Shades, Color>) -> Color {
        Color(UIColor { traits in
            UIColor(traits.userInterfaceStyle == .dark ? dark[keyPath: role] : light[keyPath: role])
        })
    }

    private static func hex(_ v: UInt32) -> Color {
        Color(
            red: Double((v >> 16) & 0xFF) / 255,
            green: Double((v >> 8) & 0xFF) / 255,
            blue: Double(v & 0xFF) / 255
        )
    }
}

extension Font {
    /// Commit shas and anything else that must line up vertically.
    static let clarityMono = Font.system(size: 13, design: .monospaced)
}
