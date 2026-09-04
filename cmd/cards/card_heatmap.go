package main

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// The calendar card: a year of contributions as a 7-row grid, one column per week.
//
// Form. One quantitative value over a two-dimensional time index, which is a
// heatmap and nothing else — a 365-column bar chart would be 365 bars two pixels
// wide, and a line would imply a continuity that daily counts do not have.
//
// Colour. Magnitude, so a sequential ramp: one hue, dark to light. Dark to light
// rather than light to dark because the surface is dark, so "more" has to mean
// "more light" or the busiest days would recede into the background. The four steps
// are spread across the ramp instead of taken adjacent from it, because at 11px a
// neighbouring pair has to separate on a glance rather than on inspection.
//
// The bands are the distribution's own quartiles, not fractions of the maximum. One
// forty-contribution day would otherwise push an entire ordinary year into the
// first band, and a chart where every cell is the same colour has measured nothing.
// The legend prints the numeric range of every band, so the reading survives with
// no colour at all.
var heatSteps = []string{trackGreen, greenRamp[4], greenRamp[2], greenRamp[0]}

// heatBands are the three cuts that split non-zero days into four buckets.
func heatBands(days []Day) [3]int {
	v := make([]int, 0, len(days))
	for _, d := range days {
		if d.Count > 0 {
			v = append(v, d.Count)
		}
	}
	return bandsOf(v)
}

// bandsOf cuts a set of non-zero counts at its quartiles. Shared by every grid on
// the profile, so two heatmaps never disagree about what a shade is worth.
func bandsOf(v []int) [3]int {
	if len(v) == 0 {
		return [3]int{1, 2, 3}
	}
	v = append([]int(nil), v...)
	sort.Ints(v)
	at := func(p float64) int { return v[int(p*float64(len(v)-1))] }
	b := [3]int{at(0.25), at(0.50), at(0.75)}

	// The cuts have to strictly increase, or two swatches would stand for the same
	// range and the legend would print an empty band. A quiet year collapses all
	// three quartiles onto 1, which is where this matters.
	if b[0] < 1 {
		b[0] = 1
	}
	for i := 1; i < 3; i++ {
		if b[i] <= b[i-1] {
			b[i] = b[i-1] + 1
		}
	}
	return b
}

// heatLevel is 0 for a quiet day and 1..4 for the four bands.
func heatLevel(count int, b [3]int) int {
	switch {
	case count <= 0:
		return 0
	case count <= b[0]:
		return 1
	case count <= b[1]:
		return 2
	case count <= b[2]:
		return 3
	default:
		return 4
	}
}

// bandLabel names a band's range in plain digits, which is what makes the legend
// readable without colour.
func bandLabel(lo, hi int) string {
	if hi <= 0 {
		return strconv.Itoa(lo) + "+"
	}
	if lo == hi {
		return strconv.Itoa(lo)
	}
	return strconv.Itoa(lo) + "–" + strconv.Itoa(hi)
}

// CalendarPeak is the calendar's one-line reading: the heaviest day of the year and
// how many days had nothing on them at all. The card and the table view both quote
// it, so it is computed once here rather than twice from the same slice.
func (s *Stats) CalendarPeak() (busiest Day, quiet int) {
	if len(s.Days) == 0 {
		return Day{}, 0
	}
	busiest = s.Days[0]
	for _, d := range s.Days {
		if d.Count > busiest.Count {
			busiest = d
		}
		if d.Count == 0 {
			quiet++
		}
	}
	return busiest, quiet
}

func renderHeatmap(s *Stats) string {
	if len(s.Days) == 0 {
		return renderEmpty("~/calendar", "No contribution calendar available yet.")
	}

	const (
		pad     = 18.0
		gutter  = 26.0 // weekday labels
		cell    = 11.0
		step    = cell + surfaceGap
		chromeH = 30.0
	)

	// Columns come from the dates rather than from the index, so a calendar with a
	// gap in it puts its cells in the right week instead of shifting everything after
	// the gap by one column.
	first := s.Days[0].Date
	weekZero := first.AddDate(0, 0, -int(first.Weekday()))
	colOf := func(d time.Time) int { return int(d.Sub(weekZero).Hours()/24) / 7 }
	cols := colOf(s.Days[len(s.Days)-1].Date) + 1

	grid := make([][7]int, cols) // -1 is "outside the year", not "a quiet day"
	dates := make([][7]time.Time, cols)
	for i := range grid {
		for j := range grid[i] {
			grid[i][j] = -1
		}
	}
	for _, d := range s.Days {
		col := colOf(d.Date)
		if col < 0 || col >= cols {
			continue
		}
		grid[col][int(d.Date.Weekday())] = d.Count
		dates[col][int(d.Date.Weekday())] = d.Date
	}
	busiest, quiet := s.CalendarPeak()

	bands := heatBands(s.Days)
	gridTop := chromeH + 22.0
	gridH := 7*step - surfaceGap
	legendY := gridTop + gridH + 28
	h := legendY + 30
	w := pad + gutter + float64(cols)*step - surfaceGap + pad

	c := newCanvas(w, h, "Contribution calendar",
		fmt.Sprintf("%s contributions over %s, one cell per day. Busiest day was %s with %s. "+
			"%s were quiet. Shading runs in four bands by daily count: %s, %s, %s and %s.",
			commas(sumCounts(s.Days)), plural(cols, "week"),
			busiest.Date.Format("2 January 2006"), plural(busiest.Count, "contribution"),
			plural(quiet, "day"),
			bandLabel(1, bands[0]), bandLabel(bands[0]+1, bands[1]),
			bandLabel(bands[1]+1, bands[2]), bandLabel(bands[2]+1, 0)))

	// Additive only: cells settle at full opacity and open at .35, so a renderer
	// that never advances the animation shows a dimmer calendar rather than an empty
	// one. The sweep is a per-week duration, never a delay.
	c.style(".hc{animation:heat 800ms ease-out}")
	c.style("@keyframes heat{from{opacity:.35}to{opacity:1}}")
	for i := 0; i < cols; i++ {
		c.style(fmt.Sprintf(".hw%d{animation-duration:%dms}", i, 560+i*14))
	}

	c.windowChrome(fmt.Sprintf("~/calendar  ·  %s contributions in %s",
		commas(sumCounts(s.Days)), plural(cols, "week")))

	// Month labels sit over the week a month opens in, so the x axis is readable
	// without counting columns.
	prev := time.Month(0)
	for col := 0; col < cols; col++ {
		var d time.Time
		for row := 0; row < 7; row++ {
			if !dates[col][row].IsZero() {
				d = dates[col][row]
				break
			}
		}
		if d.IsZero() || d.Month() == prev {
			continue
		}
		prev = d.Month()
		c.text(pad+gutter+float64(col)*step, gridTop-8, d.Format("Jan"),
			textOpts{size: 9, fill: inkLow})
	}

	// Every other weekday, which is enough to orient seven rows without stacking
	// labels into each other.
	for _, row := range []int{1, 3, 5} {
		c.text(pad+gutter-6, gridTop+float64(row)*step+cell-2.5,
			time.Weekday(row).String()[:3],
			textOpts{size: 9, fill: inkLow, anchor: "end"})
	}

	for col := 0; col < cols; col++ {
		x := pad + gutter + float64(col)*step
		for row := 0; row < 7; row++ {
			count := grid[col][row]
			if count < 0 {
				continue // outside the year: draw nothing rather than a false zero
			}
			y := gridTop + float64(row)*step
			fill := gridline
			if lvl := heatLevel(count, bands); lvl > 0 {
				fill = heatSteps[lvl-1]
			}
			c.group(fmt.Sprintf("%s — %s", dates[col][row].Format("Mon 2 Jan 2006"),
				plural(count, "contribution")), fmt.Sprintf(`class="hc hw%d"`, col))
			c.rect(x, y, cell, cell, fill, 2)
			c.groupEnd()
		}
	}

	renderHeatLegend(c, pad+gutter, legendY, "per day", bands)
	c.text(w-pad, legendY, fmt.Sprintf("busiest %s · %s",
		busiest.Date.Format("2 Jan"), plural(busiest.Count, "contribution")),
		textOpts{size: 9.5, fill: inkLow, anchor: "end"})
	caption(c, pad, h-12, w-2*pad,
		"bands are the quartiles of this year's active days, so one outlier cannot flatten the scale")
	return c.String()
}

// renderHeatLegend prints the ramp with every band's numeric range beside it. The
// numbers are the point: a sequential legend that only shows swatches asks the
// reader to guess what "darker" is worth.
func renderHeatLegend(c *canvas, x, y float64, unit string, bands [3]int) {
	c.text(x, y, unit, textOpts{size: 9.5, fill: inkLow})
	x += textWidth(unit, 9.5) + 10

	swatches := []struct {
		fill, label string
	}{
		{gridline, "0"},
		{heatSteps[0], bandLabel(1, bands[0])},
		{heatSteps[1], bandLabel(bands[0]+1, bands[1])},
		{heatSteps[2], bandLabel(bands[1]+1, bands[2])},
		{heatSteps[3], bandLabel(bands[2]+1, 0)},
	}
	for _, sw := range swatches {
		c.rect(x, y-8, 9, 9, sw.fill, 2)
		c.text(x+13, y, sw.label, textOpts{size: 9.5, fill: inkMid})
		x += 13 + textWidth(sw.label, 9.5) + 12
	}
}
