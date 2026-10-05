import SwiftUI

/**
 The type scale from the Android handoff, in the system's own faces.

 SF Pro and SF Mono rather than the bundled Roboto Flex and JetBrains Mono the
 Android app ships. The sizes, weights and roles are the handoff's; the faces
 are the platform's, because a Roboto label on iOS reads as something ported
 rather than as something built here — and SF is what every other app on the
 phone has already taught the reader to read.
 */
enum Type {
    /// The empty state's headline, and nothing else.
    static let display = Font.system(size: 34, weight: .bold)
    /// A full-screen flow's title.
    static let pageTitle = Font.system(size: 28, weight: .bold)
    static let dialogTitle = Font.system(size: 22, weight: .semibold)
    /// The repository name, and the switcher's own title.
    static let title = Font.system(size: 20, weight: .semibold)
    /// The namespace in front of a name: the same size, stepped back.
    static let titleNamespace = Font.system(size: 20, weight: .regular)
    static let appBarTitle = Font.system(size: 17, weight: .semibold)
    /// Onboarding copy, where a paragraph has to be comfortable.
    static let body = Font.system(size: 15)
    /// Secondary copy, read in passing.
    static let bodySmall = Font.system(size: 14)
    static let cardTitle = Font.system(size: 15, weight: .semibold)
    static let supporting = Font.system(size: 13)
    static let overline = Font.system(size: 11, weight: .semibold)
    static let button = Font.system(size: 15, weight: .semibold)
    /// A commit's subject: the line the list is read for.
    static let subject = Font.system(size: 15)
    /// An author, and anything that labels a row rather than carrying it.
    static let meta = Font.system(size: 12.5, weight: .medium)
    /// A band header.
    static let band = Font.system(size: 12.5, weight: .bold)
    /// A key or a URL — anything meant to be compared character by character.
    static let mono = Font.system(size: 11.5, design: .monospaced)
    /// A lead time or a branch: mono so columns line up, small so they recede.
    static let monoMeta = Font.system(size: 11.5, design: .monospaced)
}
