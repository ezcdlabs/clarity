import SwiftUI
import UIKit

/// The TUI's palette, read on a screen instead of a terminal — and the brand's,
/// where a screen can hold it.
///
/// The terminal asks for ANSI 1/2/3/8/12 and lets the emulator decide what those
/// look like. A phone has no such table, so this is it: the same roles, answered
/// twice, with the mark's own colours where they carry.
enum Ink {
    /// Two tones, and which one is the brand's matters.
    ///
    /// The chrome is the mark's navy exactly — what you see behind the top bar
    /// and around the sheet, and what the launch screen hands over to. The
    /// commits sit on a darker navy, because a reading surface should be the
    /// extreme of the pair: every role gains contrast against it.
    ///
    /// Both are the same hue and saturation at different lightnesses, derived
    /// rather than picked beside each other — what the TUI does with the
    /// terminal's real background, where a grey chosen next to a blue can only
    /// ever look pasted on to it.
    private static let dark = Shades(
        bg: hex(0x030C19),      // the brand navy at half its lightness
        surface: hex(0x061732), // the mark's navy, exactly
        line: hex(0x0C2D63),    // +11%
        text: hex(0xE6EAF2),
        dim: hex(0x8A94A8),
        red: hex(0xE06C75),
        green: hex(0x98C379),
        yellow: hex(0xF7C421),  // the mark's yellow
        blue: hex(0x6AA2FF),    // the mark's blue
        errorBg: hex(0x2E1526)
    )

    /// Light cannot take the brand's colours at their own values: the mark's
    /// yellow on white is barely a colour, and its blue is a highlight rather
    /// than something legible. What carries over is the navy, as the ink.
    private static let light = Shades(
        bg: hex(0xFCFDFF),
        surface: hex(0xEDF0F6),
        line: hex(0xDCE1EA),
        text: hex(0x061732),    // the mark's navy, as off-black
        dim: hex(0x5A6473),
        red: hex(0xC0392B),
        green: hex(0x2E7D32),
        yellow: hex(0x8A6D00),
        blue: hex(0x1565C0),
        errorBg: hex(0xFBEAEC)
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
