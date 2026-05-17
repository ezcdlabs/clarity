package core

import (
	"fmt"
	"time"
)

// FormatElapsed renders a duration as "1h 02m 03s" / "2m 05s" / "7s" form
// — the same compact form lead-time timers and "deployed Xh ago" batch
// subheaders use throughout the TUI and plain renderers.
func FormatElapsed(d time.Duration) string {
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
