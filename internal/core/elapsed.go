package core

import (
	"fmt"
	"time"
)

// FormatElapsed renders a duration as "1h 02m 03s" / "2m 05s" / "7s" form
// — the same compact form lead-time timers and "deployed Xh ago" batch
// subheaders use throughout the TUI and plain renderers.
// FormatAge renders a live "… ago" duration with precision that decays as
// magnitude grows: "37s", "4m 12s", "3h 12m", then whole days ("2d") from
// 48 hours. Precision is what makes a timer tick, so decaying it is what
// keeps an old deploy's subheader calm — second-by-second ticking on a
// week-old deploy claims a relevance it does not have. The 48h threshold
// means "1d" never appears, so a day count is never mistaken for something
// finer. Lead-time columns keep FormatElapsed instead: they are read down
// as a column, and a consistent unit shape is what makes that scannable.
func FormatAge(d time.Duration) string {
	d = d.Truncate(time.Second)
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return FormatElapsed(d)
	}
}

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
