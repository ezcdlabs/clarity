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
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.DrawScope
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
    // Read off the payload before the chart is drawn, so the window's limits
    // are available without the compiler having to prove that a flow implies a
    // payload.
    val truncated = metrics?.truncated == true
    val commitLimit = metrics?.limit ?: 0
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
                LazyColumn(Modifier.weight(1f)) {
                    items(flow.weeksCount, key = { i -> flow.getWeeks(i).label }) { i ->
                        WeekRow(flow.getWeeks(i), flow)
                    }
                    if (truncated) {
                        item { TruncationNotice(commitLimit) }
                    }
                }
                // Pinned rather than placed after the last row, which is where
                // the terminal puts it: a terminal shows every row at once, and
                // a scale you have to scroll to is a scale you cannot read the
                // rows against.
                Axis(flow)
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
private val boxHeight = 11.dp

/** The mark that says a week runs past the end of the scale. */
private val clampWidth = 7.dp

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
 */
@Composable
private fun LeadPlot(week: Week, axisMaxSeconds: Long) {
    val dim = Ink.dim
    val blue = Ink.blue
    val clamp = Ink.yellow
    Canvas(Modifier.fillMaxSize()) {
        // The mark takes the end of the scale and the plot draws into what is
        // left, rather than being drawn over: at this width an overlaid mark
        // would cover whatever landed under it, including the median.
        val clampPx = if (week.beyondAxis) clampWidth.toPx() else 0f
        val span = size.width - clampPx
        if (span <= 0f) return@Canvas

        fun x(seconds: Long): Float =
            if (axisMaxSeconds <= 0) 0f
            else (seconds.toFloat() / axisMaxSeconds.toFloat()).coerceIn(0f, 1f) * span

        val mid = size.height / 2f
        val box = boxHeight.toPx()

        if (week.plot == Plot.PLOT_POINTS) {
            for (s in week.sampleSecondsList) {
                drawCircle(blue, radius = box / 4f, center = Offset(x(s), mid))
            }
        } else {
            // Whiskers to the extremes. Spread matters as much as the middle: a
            // two-hour median with a three-day p75 is a problem the median
            // alone hides.
            drawLine(
                dim,
                Offset(x(week.minSeconds), mid),
                Offset(x(week.maxSeconds), mid),
                strokeWidth = 1.dp.toPx(),
            )
            for (end in listOf(week.minSeconds, week.maxSeconds)) {
                drawLine(
                    dim,
                    Offset(x(end), mid - box / 2.6f),
                    Offset(x(end), mid + box / 2.6f),
                    strokeWidth = 1.dp.toPx(),
                )
            }
            val left = x(week.p25Seconds)
            val right = x(week.p75Seconds)
            drawRoundedBox(blue.copy(alpha = 0.38f), left, right, mid, box)
            // The median is the headline, not the mean: lead times are strongly
            // right-skewed, so one commit that sat over a weekend drags an
            // average badly.
            drawLine(
                blue,
                Offset(x(week.p50Seconds), mid - box / 2f),
                Offset(x(week.p50Seconds), mid + box / 2f),
                strokeWidth = 2.5.dp.toPx(),
                cap = StrokeCap.Round,
            )
        }

        if (week.beyondAxis) {
            drawClampMark(clamp, size.width, mid, clampPx)
        }
    }
}

/** The interquartile range, with a minimum so a tight week is still a shape. */
private fun DrawScope.drawRoundedBox(colour: Color, left: Float, right: Float, mid: Float, box: Float) {
    val minimum = box / 2.5f
    val width = max(right - left, minimum)
    drawRoundRect(
        colour,
        topLeft = Offset(left, mid - box / 2f),
        size = Size(width, box),
        cornerRadius = androidx.compose.ui.geometry.CornerRadius(2.5f.dp.toPx()),
    )
}

/** A small chevron, for a week with a lead time past the end of the scale. */
private fun DrawScope.drawClampMark(colour: Color, rightEdge: Float, mid: Float, room: Float) {
    if (room <= 0f) return
    val tip = rightEdge - 1.dp.toPx()
    val back = tip - room / 2f
    val arm = room / 2f
    drawLine(colour, Offset(back, mid - arm), Offset(tip, mid), strokeWidth = 1.5.dp.toPx(), cap = StrokeCap.Round)
    drawLine(colour, Offset(back, mid + arm), Offset(tip, mid), strokeWidth = 1.5.dp.toPx(), cap = StrokeCap.Round)
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
 * The shared scale, labelled.
 *
 * Every label crosses the boundary; how many of them fit does not. A phone is
 * narrow in a different way from a terminal, so this keeps the ends and drops
 * the quarters rather than the terminal's three-step degradation — and the
 * labels themselves come from Go, so a half hour reads "1.5h" here exactly as
 * it does there.
 */
@Composable
private fun Axis(flow: MetricsFlow) {
    val ticks = flow.axis.ticksList
    if (ticks.isEmpty()) return
    Rule()
    Row(
        Modifier.fillMaxWidth()
            .padding(start = PageMargin, end = PageMargin, top = 2.dp, bottom = 14.dp),
    ) {
        Spacer(Modifier.width(weekLabelWidth))
        Box(Modifier.weight(PLOT_WEIGHT)) {
            // Laid out by fraction rather than spaced evenly, so a label sits
            // over the place on the scale it names.
            ticks.forEachIndexed { i, tick ->
                val keep = i == 0 || i == ticks.size - 1 || i == ticks.size / 2
                if (keep) {
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
        Spacer(Modifier.width(columnGap))
        Spacer(Modifier.weight(BAR_WEIGHT))
        Spacer(Modifier.width(countWidth))
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
 * What the commit window cut short.
 *
 * Worth more here than in the commit list, which already says it: a reader can
 * see the bottom of a scroll, but nothing on a chart says it is short a few
 * deploys, and an aggregate missing some is wrong in a way that looks right.
 */
@Composable
private fun TruncationNotice(limit: Int) {
    Text(
        "Read the most recent $limit commits — weeks before that are not shown.",
        style = Type.supporting,
        color = Ink.dim,
        modifier = Modifier.fillMaxWidth().padding(PageMargin),
    )
}
