package dev.ezcd.clarity.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.layout.layout
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.ezcd.clarity.AppState
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.proto.MetricsFlow
import dev.ezcd.clarity.proto.Plot
import dev.ezcd.clarity.proto.Week
import dev.ezcd.clarity.title
import kotlin.math.max
import kotlin.math.roundToInt

/**
 * The weekly aggregates — the phone's `git clarity metrics`.
 *
 * The commit feed asks "is main green right now?"; this asks "are we getting
 * better?", which is invisible in a list of commits because trend needs rows to
 * compare. One row per ISO week: a box plot of that week's lead times on the
 * left, a bar of its deploy count on the right.
 *
 * The two halves are one signal, and the direction is deliberate. Bars share a
 * right edge and grow leftward, so a bad week pushes the box right AND pulls
 * the bar back — degradation reads as ink migrating one way across the row.
 * Bars anchored left would move the two halves apart instead, leaving the
 * reader tracking two opposing signals. That is the terminal's reasoning and it
 * is about meaning rather than about character cells, so it carries.
 *
 * What does not carry is any of the terminal's quantisation: no cell was
 * reserved for the clamp mark, no bar was forced to a one-cell minimum, and the
 * axis sheds labels on a different rule because a phone is narrow in a
 * different way.
 */
@Composable
fun MetricsScreen(state: AppState, model: ClarityModel) {
    val metrics = state.metrics
    val flows = metrics?.flowsList.orEmpty()
    // Read off the payload before the chart is drawn, so it is available
    // without the compiler having to prove that a flow implies a payload.
    val truncated = metrics?.truncated == true
    val tab = state.metricsFlow.coerceIn(0, max(flows.size - 1, 0))
    val flow = flows.getOrNull(tab)

    Column(Modifier.fillMaxSize().background(Ink.surface)) {
        MetricsBar(state, model)
        if (flows.size > 1) {
            FlowTabs(flows, tab) { model.showMetrics(it) }
        }

        Box(Modifier.fillMaxSize().background(Ink.bg)) {
            Column(Modifier.fillMaxSize()) {
                ErrorBar(state.error) { model.dismissError() }

                if (flow == null || flow.weeksCount == 0) {
                    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Text(
                            when {
                                state.busy -> "Reading the history…"
                                state.error != null -> ""
                                else -> "No deploys recorded yet."
                            },
                            style = Type.bodySmall,
                            color = Ink.dim,
                        )
                    }
                    return@Column
                }

                Legend()
                // fill = false, so the list is only as tall as its rows and the
                // scale follows them. A few weeks should not leave the axis
                // stranded at the foot of an empty screen, and a year of them
                // should not push it off the bottom — this gives the first case
                // a scale under the rows and the second a scale pinned below a
                // list that scrolls under it.
                LazyColumn(Modifier.weight(1f, fill = false)) {
                    items(flow.weeksCount, key = { i -> flow.getWeeks(i).label }) { i ->
                        WeekRow(flow.getWeeks(i), flow)
                    }
                }
                Axis(flow)
                if (truncated) {
                    TruncationNotice()
                }
            }
        }
    }
}

/**
 * The bar: a back arrow, what this is, and which repository it is about.
 *
 * Deliberately not the feed's bar. That one carries ci and deploy marks, which
 * answer the other screen's whole question — and here they would be frozen at
 * the moment this screen opened, because nothing on it polls. A mark that looks
 * live and is not is worse than no mark.
 */
@Composable
private fun MetricsBar(state: AppState, model: ClarityModel) {
    Row(
        Modifier.fillMaxWidth().height(64.dp).padding(end = PageMargin),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        GlyphButton(Icons.AutoMirrored.Rounded.ArrowBack, "Back") { model.closeOverlay() }
        Column(Modifier.weight(1f)) {
            Text("Metrics", style = Type.appBarTitle, color = Ink.text)
            state.repo?.let {
                Text(
                    it.title,
                    style = Type.mono,
                    color = Ink.dim,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/** One chip per deploy target, as the feed has. */
@Composable
private fun FlowTabs(flows: List<MetricsFlow>, tab: Int, onPick: (Int) -> Unit) {
    Row(
        Modifier.fillMaxWidth()
            .horizontalScroll(rememberScrollState())
            .padding(start = PageMargin, top = 4.dp),
    ) {
        flows.forEachIndexed { i, f ->
            val selected = i == tab
            Row(
                Modifier
                    .clip(RoundedCornerShape(topStart = 12.dp, topEnd = 12.dp))
                    .background(if (selected) Ink.bg else Color.Transparent)
                    .clickable { onPick(i) }
                    .padding(start = 12.dp, end = 12.dp, top = 10.dp, bottom = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    if (f.undeclared) "${f.name} ?" else f.name,
                    style = Type.monoMeta,
                    color = if (selected) Ink.text else Ink.dim,
                )
            }
        }
    }
}

/**
 * The grid every row shares.
 *
 * The plot gets the larger share: the lead time distribution is the headline,
 * and a bar only has to be comparable to the bars above and below it.
 */
private val weekLabelWidth = 62.dp
// Four figures, as the terminal reserves: the column must not reflow when a
// busy week lands in it, because the rows are read against each other.
private val countWidth = 34.dp
private val columnGap = 10.dp
private const val PLOT_WEIGHT = 2f
private const val BAR_WEIGHT = 1f

/** The row height a box plot and a count sit comfortably in. */
private val rowHeight = 26.dp

/** How tall the interquartile box is drawn, within [rowHeight]. */
private val boxHeight = 12.dp

/** The mark that says a week runs past the end of the scale. */
private val clampWidth = 7.dp

/** How far the axis ticks drop below the rule. */
private val tickHeight = 4.dp

@Composable
private fun Legend() {
    Row(
        Modifier.fillMaxWidth().padding(start = PageMargin, end = PageMargin, top = 18.dp),
        verticalAlignment = Alignment.Bottom,
    ) {
        Spacer(Modifier.width(weekLabelWidth))
        Text(
            "lead time · median, p25–p75",
            style = Type.monoMeta,
            color = Ink.dim,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        Text("deploys", style = Type.monoMeta, color = Ink.dim)
    }
    Rule()
    Spacer(Modifier.height(6.dp))
}

@Composable
private fun WeekRow(week: Week, flow: MetricsFlow) {
    Row(
        Modifier.fillMaxWidth().height(rowHeight).padding(horizontal = PageMargin),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            week.label,
            style = Type.monoMeta,
            // A week nothing shipped in is still a row — a gap in the trend is
            // a finding — but it is not the row you are reading.
            color = if (week.deploys == 0) Ink.line else Ink.dim,
            maxLines = 1,
            modifier = Modifier.width(weekLabelWidth),
        )

        Box(Modifier.weight(PLOT_WEIGHT).fillMaxSize(), contentAlignment = Alignment.CenterStart) {
            if (week.plot == Plot.PLOT_NONE) {
                Text("no deploys", style = Type.monoMeta, color = Ink.line, maxLines = 1)
            } else {
                LeadPlot(week, flow.axis.maxSeconds)
            }
        }

        Spacer(Modifier.width(columnGap))

        DeployBar(
            week.deploys,
            flow.maxDeploys,
            Modifier.weight(BAR_WEIGHT).height(boxHeight),
        )
        Text(
            week.deploys.toString(),
            style = Type.monoMeta,
            color = if (week.deploys == 0) Ink.line else Ink.text,
            maxLines = 1,
            textAlign = TextAlign.End,
            modifier = Modifier.width(countWidth).padding(start = 6.dp),
        )
    }
}

/**
 * One week's lead times against the shared scale.
 *
 * A box plot where there are enough deploys for quartiles to mean anything, and
 * the individual deploys where there are not. Which of the two is the core's
 * decision and arrives in [Week.getPlot]: below the floor the quartiles are
 * interpolations between two or three real values, so drawing them would put a
 * smear on screen where there were only two deploys.
 *
 * Drawn the way a box plot actually looks: a hairline whisker with square caps,
 * a stroked rectangle across the interquartile range, and one crisp rule at the
 * median. The terminal fills its box with ▓ and caps its median with █ because
 * those are the marks a character cell can hold — a half-shaded block is what a
 * box and a fill look like when the smallest unit you have is a letter. None of
 * that is a decision worth carrying to a surface that can draw a hairline.
 */
@Composable
private fun LeadPlot(week: Week, axisMaxSeconds: Long) {
    val hairline = Ink.dim
    val blue = Ink.blue
    val clamp = Ink.yellow
    Canvas(Modifier.fillMaxSize()) {
        // The chevron's room is reserved on every row, not only the rows that
        // use it. The terminal takes a column back from the clamped row alone,
        // which it can afford because a row is read against the rows above it;
        // here the rows are read against a scale drawn once underneath them,
        // and a row that quietly rescaled itself by seven points would put its
        // median in a different place from an identical median next to it.
        //
        // Reserved rather than drawn over, for the terminal's reason: an
        // overlaid mark covers whatever landed under it, including the median.
        val clampPx = clampWidth.toPx()
        val span = size.width - clampPx
        if (span <= 0f) return@Canvas

        fun x(seconds: Long): Float =
            if (axisMaxSeconds <= 0) {
                0f
            } else {
                (seconds.toFloat() / axisMaxSeconds.toFloat()).coerceIn(0f, 1f) * span
            }

        val mid = size.height / 2f
        val box = boxHeight.toPx()
        val hair = 1.dp.toPx()
        val whisker = box * 0.52f

        if (week.plot == Plot.PLOT_POINTS) {
            // Hollow, as the terminal's ▫ is: these are the individual deploys
            // rather than a summary, and an outline says "one of these" where a
            // filled dot reads as a quantity.
            for (sec in week.sampleSecondsList) {
                drawCircle(
                    blue,
                    radius = box / 3.4f,
                    center = Offset(x(sec), mid),
                    style = Stroke(width = 1.4.dp.toPx()),
                )
            }
        } else {
            val lo = x(week.minSeconds)
            val hi = x(week.maxSeconds)
            val q1 = x(week.p25Seconds)
            val q3 = x(week.p75Seconds)

            // Whiskers to the extremes, with square caps. Spread matters as
            // much as the middle: a two-hour median with a three-day p75 is a
            // problem the median alone hides.
            drawLine(hairline, Offset(lo, mid), Offset(hi, mid), strokeWidth = hair)
            for (end in listOf(lo, hi)) {
                drawLine(
                    hairline,
                    Offset(end, mid - whisker / 2f),
                    Offset(end, mid + whisker / 2f),
                    strokeWidth = hair,
                )
            }

            // The interquartile range: a stroked rectangle over a wash, so its
            // edges are the quartiles rather than the edges of a blob. Floored
            // at a couple of hairs so a week where everything shipped within
            // ten minutes is still a box rather than a line.
            val width = max(q3 - q1, hair * 2f)
            drawRect(
                blue.copy(alpha = 0.16f),
                topLeft = Offset(q1, mid - box / 2f),
                size = Size(width, box),
            )
            drawRect(
                blue,
                topLeft = Offset(q1, mid - box / 2f),
                size = Size(width, box),
                style = Stroke(width = 1.2.dp.toPx()),
            )

            // The median is the headline, not the mean: lead times are strongly
            // right-skewed, so one commit that sat over a weekend drags an
            // average badly. One rule, the height of the box and no taller — a
            // cap that overhangs reads as a second mark.
            drawLine(
                blue,
                Offset(x(week.p50Seconds), mid - box / 2f),
                Offset(x(week.p50Seconds), mid + box / 2f),
                strokeWidth = 2.dp.toPx(),
            )
        }

        if (week.beyondAxis) {
            drawClampMark(clamp, size.width, mid, clampPx, whisker)
        }
    }
}

/**
 * A chevron, for a week with a lead time past the end of the scale.
 *
 * Sized to the whisker it continues rather than to the row, so it reads as the
 * line carrying on off the edge rather than as a separate symbol parked there.
 */
private fun DrawScope.drawClampMark(
    colour: Color,
    rightEdge: Float,
    mid: Float,
    room: Float,
    whisker: Float,
) {
    if (room <= 0f) return
    val tip = rightEdge - 1.dp.toPx()
    val back = tip - room * 0.6f
    val arm = whisker / 2f
    drawLine(colour, Offset(back, mid - arm), Offset(tip, mid), strokeWidth = 1.4.dp.toPx())
    drawLine(colour, Offset(back, mid + arm), Offset(tip, mid), strokeWidth = 1.4.dp.toPx())
}

/**
 * A week's deploy count, relative to the busiest week on show.
 *
 * Right-aligned and growing leftward, which is the half of the row's signal
 * that makes it one signal. A week with one deploy still gets a visible sliver:
 * zero width and "one deploy" would be indistinguishable from "none", and the
 * count beside it is the only other thing that could tell them apart.
 */
@Composable
private fun DeployBar(deploys: Int, maxDeploys: Int, modifier: Modifier = Modifier) {
    val colour = Ink.dim
    Canvas(modifier) {
        if (deploys <= 0 || maxDeploys <= 0) return@Canvas
        val minimum = 2.dp.toPx()
        val width = max(size.width * deploys.toFloat() / maxDeploys.toFloat(), minimum)
        drawRoundRect(
            colour,
            topLeft = Offset(size.width - width, 0f),
            size = Size(width, size.height),
            cornerRadius = androidx.compose.ui.geometry.CornerRadius(2.5f.dp.toPx()),
        )
    }
}

/**
 * The shared scale: a rule, a tick at every fraction, and the labels that fit.
 *
 * Every label crosses the boundary; how many of them to draw does not. A phone
 * is narrow in a different way from a terminal, so this keeps the ends and the
 * midpoint and drops the quarters, rather than the terminal's three-step
 * degradation — but the ticks themselves stay at all five, because a mark costs
 * a hairline and knowing where the quarters fall is most of what a scale is
 * for. The labels come from Go, so a half hour reads "1.5h" here exactly as it
 * does there.
 */
@Composable
private fun Axis(flow: MetricsFlow) {
    val ticks = flow.axis.ticksList
    if (ticks.isEmpty()) return
    Row(
        Modifier.fillMaxWidth().padding(start = PageMargin, end = PageMargin, top = 6.dp),
    ) {
        Spacer(Modifier.width(weekLabelWidth))
        // Inset by the same room every row reserves for its clamp chevron, so
        // the scale spans exactly the range the plots are drawn into.
        Column(Modifier.weight(PLOT_WEIGHT).padding(end = clampWidth)) {
            TickRule(ticks.map { it.fraction.toFloat() })
            Box(Modifier.fillMaxWidth().padding(top = 3.dp)) {
                ticks.forEachIndexed { i, tick ->
                    if (labelled(i, ticks.size)) {
                        Text(
                            tick.label,
                            style = Type.monoMeta,
                            color = Ink.dim,
                            maxLines = 1,
                            modifier = Modifier.offsetByFraction(tick.fraction.toFloat()),
                        )
                    }
                }
            }
        }
        Spacer(Modifier.width(columnGap))
        Spacer(Modifier.weight(BAR_WEIGHT))
        Spacer(Modifier.width(countWidth))
    }
    Spacer(Modifier.height(14.dp))
}

/** The ends and the middle, which is as many labels as this width holds. */
private fun labelled(i: Int, count: Int): Boolean = i == 0 || i == count - 1 || i == count / 2

/**
 * The axis line, with a tick dropped at each fraction.
 *
 * The end ticks are pulled inside by half their width so neither hangs off the
 * rule it belongs to — the same reason the first and last labels are pulled in.
 */
@Composable
private fun TickRule(fractions: List<Float>) {
    val colour = Ink.line
    Canvas(Modifier.fillMaxWidth().height(tickHeight)) {
        val hair = 1.dp.toPx()
        drawLine(colour, Offset(0f, 0f), Offset(size.width, 0f), strokeWidth = hair)
        for (fraction in fractions) {
            val x = (size.width - hair) * fraction + hair / 2f
            drawLine(colour, Offset(x, 0f), Offset(x, size.height), strokeWidth = hair)
        }
    }
}

/**
 * Places a label at a fraction of the width, pulled inside at both ends.
 *
 * The first label would otherwise start at the axis origin and look like it
 * belongs to the week column, and the last would run off the right edge.
 */
private fun Modifier.offsetByFraction(fraction: Float): Modifier = layout { measurable, constraints ->
    val placeable = measurable.measure(constraints.copy(minWidth = 0))
    val room = constraints.maxWidth - placeable.width
    layout(constraints.maxWidth, placeable.height) {
        placeable.placeRelative((room * fraction).roundToInt().coerceIn(0, max(room, 0)), 0)
    }
}

/**
 * What the window cut short.
 *
 * Worth more here than in the commit list, which already says it: a reader can
 * see the bottom of a scroll, but nothing on a chart says it is short a few
 * deploys, and an aggregate missing some is wrong in a way that looks right.
 *
 * It names no number, deliberately. Two different things end the history — the
 * commit window this screen reads over, and how much of the branch has ever
 * been fetched to the device — and the snapshot reports them as one flag. An
 * earlier version named the commit window and was usually wrong: a shallow
 * clone is what runs out first, and no limit can read past it.
 */
@Composable
private fun TruncationNotice() {
    Text(
        "The repository goes back further than the history on this device.",
        style = Type.supporting,
        color = Ink.dim,
        modifier = Modifier.fillMaxWidth().padding(horizontal = PageMargin, vertical = 10.dp),
    )
}
