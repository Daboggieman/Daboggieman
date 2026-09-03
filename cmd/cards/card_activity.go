package main

import "fmt"

// statTile is one readout in the KPI row: label + value, no plot of its own.
type statTile struct {
	label string
	value string
}

// renderActivity draws the activity card: one hero figure, a KPI row, and a
// 30-day contribution sparkline.
//
// A handful of headline numbers is a KPI row of stat tiles, not a grouped bar
// chart. Direction on the delta is carried by a glyph and a sign rather than
// color, so it never leans on hue and never borrows a reserved status color.
func renderActivity(s *Stats) string {
	const (
		w        = 460.0
		pad      = 18.0
		sparkDay = 30
		sparkH   = 34.0
	)

	chromeH := 30.0
	c := newCanvas(w, 0, "Contribution activity",
		fmt.Sprintf("%d contributions in the last 365 days; %d day current streak; %d day longest streak.",
			s.TotalContributions, s.CurrentStreak, s.LongestStreak))

	// --- hero figure: the one number the card leads with ---
	heroLabelY := chromeH + 24.0
	heroValueY := heroLabelY + 40.0

	recent := lastDays(s.Days, sparkDay)
	prior := priorWindow(s.Days, sparkDay)
	deltaText := formatDelta(sumCounts(recent), sumCounts(prior), sparkDay)

	// --- KPI row ---
	tiles := []statTile{
		{"current streak", fmt.Sprintf("%dd", s.CurrentStreak)},
		{"longest streak", fmt.Sprintf("%dd", s.LongestStreak)},
		{"commits", compact(s.Commits)},
		{"stars earned", compact(s.Stars)},
	}
	tileTop := heroValueY + 26.0
	tileH := 40.0
	sparkTop := tileTop + tileH + 26.0
	h := sparkTop + sparkH + 30.0
	c.h = h

	c.windowChrome("~/activity  ·  last 365 days")

	c.text(pad, heroLabelY, "contributions", textOpts{size: 10, fill: inkLow})
	c.text(pad, heroValueY, compact(s.TotalContributions), textOpts{
		size: 40, fill: inkHi, weight: "700",
		tooltip: fmt.Sprintf("%d contributions in the last 365 days", s.TotalContributions),
	})
	if deltaText != "" {
		c.text(pad+textWidth(compact(s.TotalContributions), 40)+12, heroValueY-2, deltaText,
			textOpts{size: 10.5, fill: inkMid})
	}

	c.hRule(pad, w-pad, tileTop-14, gridline)

	colW := (w - 2*pad) / float64(len(tiles))
	for i, t := range tiles {
		x := pad + float64(i)*colW
		c.text(x, tileTop+4, t.label, textOpts{size: 9.5, fill: inkLow})
		c.text(x, tileTop+26, t.value, textOpts{size: 17, fill: inkHi, weight: "500"})
	}

	renderSparkline(c, recent, pad, sparkTop, w-2*pad, sparkH)
	return c.String()
}

// renderSparkline draws daily contribution columns. Spent days recede in the
// dimmed accent; the current day carries the accent, per the stat-tile trend
// contract.
func renderSparkline(c *canvas, days []Day, x, y, w, h float64) {
	c.text(x, y-8, fmt.Sprintf("last %d days", len(days)), textOpts{size: 9.5, fill: inkLow})
	if len(days) == 0 {
		return
	}

	c.hRule(x, x+w, y+h, gridline)

	peak := float64(maxCount(days))
	pitch := w / float64(len(days))
	barW := pitch - surfaceGap // the 2px gap is surface, not a stroke
	if barW < 1 {
		barW = 1
	}
	if barW > barThickMax {
		barW = barThickMax
	}

	for i, d := range days {
		bh := (float64(d.Count) / peak) * h
		fill := accentDim
		if i == len(days)-1 {
			fill = accent
		}
		if d.Count == 0 {
			// A quiet day still gets a visible seat on the axis.
			c.rect(x+float64(i)*pitch, y+h-1.5, barW, 1.5, gridline, 0)
			continue
		}
		if bh < 2 {
			bh = 2
		}
		c.vBar(x+float64(i)*pitch, y+h-bh, barW, bh, fill,
			fmt.Sprintf("%s — %s", d.Date.Format("Mon 2 Jan"), plural(d.Count, "contribution")))
	}

	c.text(x, y+h+14, days[0].Date.Format("2 Jan"), textOpts{size: 9, fill: inkLow})
	c.text(x+w, y+h+14, days[len(days)-1].Date.Format("2 Jan"),
		textOpts{size: 9, fill: inkLow, anchor: "end"})
	c.text(x+w, y-8, fmt.Sprintf("peak %s", plural(maxCount(days), "contribution")),
		textOpts{size: 9.5, fill: inkLow, anchor: "end"})
}
