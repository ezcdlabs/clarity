package dev.ezcd.clarity.ui

import androidx.compose.ui.text.ExperimentalTextApi
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import dev.ezcd.clarity.R

/**
 * The type scale from the Android handoff.
 *
 * Roboto Flex is a variable font, and the weights the design asks for — 500,
 * 600, 650 — exist as points on its weight axis rather than as separate files.
 * Android only applies variation settings from API 26, so on Android 7, which
 * minSdk still admits, every weight collapses to regular. That is a legible
 * app with flat emphasis on a vanishing share of devices, which is a better
 * trade than dropping those devices for a typeface.
 */
@OptIn(ExperimentalTextApi::class)
private fun flex(weight: Int) = Font(
    R.font.roboto_flex,
    weight = FontWeight(weight),
    variationSettings = FontVariation.Settings(FontVariation.weight(weight)),
)

private val Flex = FontFamily(flex(400), flex(500), flex(600), flex(650), flex(700))

/** JetBrains Mono, for anything copyable or code-like. Regular only: the
 *  design never asks mono for emphasis, and a second weight is 270KB. */
val MonoFamily = FontFamily(Font(R.font.jetbrains_mono, FontWeight.Normal))

/**
 * Named for what each style is *for* rather than for how big it is, so a screen
 * reads as a layout decision and a size change happens in one place.
 */
object Type {
    /** The empty state's headline, and nothing else. */
    val display = TextStyle(
        fontFamily = Flex, fontSize = 34.sp, lineHeight = 40.sp,
        fontWeight = FontWeight(650), letterSpacing = (-0.8).sp,
    )

    /** A full-screen flow's title, as on Connect a repository. */
    val pageTitle = TextStyle(
        fontFamily = Flex, fontSize = 28.sp, lineHeight = 34.sp,
        fontWeight = FontWeight(650), letterSpacing = (-0.6).sp,
    )

    val dialogTitle = TextStyle(
        fontFamily = Flex, fontSize = 22.sp, lineHeight = 28.sp,
        fontWeight = FontWeight(600), letterSpacing = (-0.3).sp,
    )

    /** The repository name in the bar, and the switcher's own title. */
    val title = TextStyle(
        fontFamily = Flex, fontSize = 20.sp,
        fontWeight = FontWeight(600), letterSpacing = (-0.3).sp,
    )

    /** The namespace in front of a name: the same size, stepped back. */
    val titleNamespace = title.copy(fontWeight = FontWeight(400))

    val appBarTitle = TextStyle(
        fontFamily = Flex, fontSize = 17.sp,
        fontWeight = FontWeight(600), letterSpacing = (-0.2).sp,
    )

    /** Onboarding copy, where a paragraph has to be comfortable. */
    val body = TextStyle(fontFamily = Flex, fontSize = 15.sp, lineHeight = 22.sp)

    /** Secondary copy, tighter because it is read in passing. */
    val bodySmall = TextStyle(fontFamily = Flex, fontSize = 14.sp, lineHeight = 20.sp)

    val cardTitle = TextStyle(fontFamily = Flex, fontSize = 15.sp, fontWeight = FontWeight(600))

    val supporting = TextStyle(fontFamily = Flex, fontSize = 13.sp, lineHeight = 18.sp)

    val overline = TextStyle(
        fontFamily = Flex, fontSize = 11.sp, lineHeight = 14.sp,
        fontWeight = FontWeight(600), letterSpacing = 1.2.sp,
    )

    val button = TextStyle(fontFamily = Flex, fontSize = 15.sp, fontWeight = FontWeight(600))
    val buttonSmall = TextStyle(fontFamily = Flex, fontSize = 14.sp, fontWeight = FontWeight(600))

    /** A commit's subject: the line the list is read for. */
    val subject = TextStyle(fontFamily = Flex, fontSize = 15.sp, lineHeight = 20.sp)

    /** An author, and anything else that labels a row rather than carrying it. */
    val meta = TextStyle(fontFamily = Flex, fontSize = 12.5f.sp, fontWeight = FontWeight(500))

    /** A band header. */
    val band = TextStyle(
        fontFamily = Flex, fontSize = 12.5f.sp,
        fontWeight = FontWeight(700), letterSpacing = 0.2.sp,
    )

    /** A key, a fingerprint, a URL — anything meant to be compared character
     *  by character. */
    val mono = TextStyle(fontFamily = MonoFamily, fontSize = 11.5f.sp, lineHeight = 17.sp)

    /** A lead time or a branch: mono so columns line up, small so they recede. */
    val monoMeta = TextStyle(fontFamily = MonoFamily, fontSize = 11.5f.sp)
}
