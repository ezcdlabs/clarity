import SwiftUI

/// The TUI's palette, read on a screen instead of a terminal.
///
/// The terminal asks for ANSI 1/2/3/8/12 and lets the emulator decide what those
/// look like. A phone has no such table, so these are the same six roles pinned
/// to values that stay legible on an OLED panel — and the same values the
/// Android app uses, so the two are one product.
enum Ink {
    static let bg = Color(red: 0x10 / 255, green: 0x10 / 255, blue: 0x10 / 255)
    static let surface = Color(red: 0x1A / 255, green: 0x1A / 255, blue: 0x1A / 255)
    static let line = Color(red: 0x2A / 255, green: 0x2A / 255, blue: 0x2A / 255)
    static let text = Color(red: 0xE6 / 255, green: 0xE6 / 255, blue: 0xE6 / 255)

    /// ANSI 8. Everything secondary in both UIs is this.
    static let dim = Color(red: 0x80 / 255, green: 0x80 / 255, blue: 0x80 / 255)

    static let red = Color(red: 0xE0 / 255, green: 0x6C / 255, blue: 0x75 / 255)
    static let green = Color(red: 0x98 / 255, green: 0xC3 / 255, blue: 0x79 / 255)
    static let yellow = Color(red: 0xE5 / 255, green: 0xC0 / 255, blue: 0x7B / 255)
    static let blue = Color(red: 0x61 / 255, green: 0xAF / 255, blue: 0xEF / 255)
}

extension Font {
    /// Commit shas and anything else that must line up vertically.
    static let clarityMono = Font.system(size: 13, design: .monospaced)
}
