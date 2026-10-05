package dev.ezcd.clarity.ui

import androidx.compose.foundation.Image
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.AddLink
import androidx.compose.material.icons.rounded.Key
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import dev.ezcd.clarity.ClarityModel
import dev.ezcd.clarity.R
import dev.ezcd.clarity.proto.Status

/**
 * First open, with nothing connected.
 *
 * It answers "what is this for?" before asking for anything, which is the only
 * screen in the app with room to. The legend is the three bands the feed is
 * made of, so the first repository that appears is already readable.
 */
@Composable
fun EmptyScreen(model: ClarityModel) {
    Column(Modifier.fillMaxSize()) {
        Row(
            Modifier.fillMaxWidth().padding(start = PageMargin, end = 4.dp).height(64.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Image(
                painterResource(R.mipmap.splash_mark),
                contentDescription = null,
                modifier = Modifier.size(28.dp).clip(androidx.compose.foundation.shape.CircleShape),
            )
            Text(
                "Git Clarity",
                style = Type.appBarTitle,
                color = Ink.text,
                modifier = Modifier.weight(1f).padding(start = 10.dp),
            )
            Overflow(model)
        }

        Column(
            Modifier.weight(1f).fillMaxWidth().padding(horizontal = 24.dp),
            verticalArrangement = Arrangement.Center,
        ) {
            Text("NO REPOSITORIES YET", style = Type.overline, color = Ink.dim)
            Spacer(Modifier.height(12.dp))
            Text("Is main green?", style = Type.display, color = Ink.text)
            Spacer(Modifier.height(16.dp))
            Text(
                "Connect a repository to see what just landed, what passed CI " +
                    "and what's live in production. It's the same view as " +
                    "git clarity in your terminal.",
                style = Type.body,
                color = Ink.dim,
            )
            Spacer(Modifier.height(28.dp))

            Legend(Status.STATUS_NONE, "HEAD", Ink.text, "what just landed")
            Spacer(Modifier.height(14.dp))
            Legend(Status.STATUS_PASSED, "CI Passed", Ink.yellow, "green, waiting to ship")
            Spacer(Modifier.height(14.dp))
            Legend(Status.STATUS_PASSED, "Deployed", Ink.blue, "live, with lead times")
        }

        Column(Modifier.fillMaxWidth().padding(24.dp)) {
            PrimaryButton(
                "Connect a repository",
                icon = Icons.Rounded.AddLink,
                modifier = Modifier.fillMaxWidth(),
            ) { model.showConnect() }
            Spacer(Modifier.height(12.dp))
            Text(
                "Any git remote over SSH. Read-only.",
                style = Type.supporting,
                color = Ink.dim,
                textAlign = TextAlign.Center,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}

/** One band, as it will look in the feed, with what it means beside it. */
@Composable
private fun Legend(status: Status, band: String, colour: Color, meaning: String) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.width(16.dp), contentAlignment = Alignment.Center) {
            StatusGlyph(status, size = 16)
        }
        Text(
            band,
            style = Type.supporting.copy(fontWeight = androidx.compose.ui.text.font.FontWeight(600)),
            color = colour,
            modifier = Modifier.padding(start = 10.dp).width(76.dp),
        )
        Text(meaning, style = Type.supporting, color = Ink.dim)
    }
}

/** The overflow that holds the one action with nowhere else to live. */
@Composable
fun Overflow(model: ClarityModel) {
    var open by remember { mutableStateOf(false) }
    Box {
        GlyphButton(Icons.Rounded.MoreVert, "More") { open = true }
        DropdownMenu(
            expanded = open,
            onDismissRequest = { open = false },
            containerColor = Ink.menu,
        ) {
            DropdownMenuItem(
                text = { Text("Device key", style = Type.bodySmall, color = Ink.text) },
                leadingIcon = { Icon(Icons.Rounded.Key, null, tint = Ink.dim) },
                onClick = {
                    open = false
                    model.showKey()
                },
            )
        }
    }
}
