package main

import (
	"fmt"
	"time"
)

// The year grid: the whole record at once, years down the side and months across.
//
// Form. The calendar card answers "what did this year look like"; this one answers
// a question no single year can — whether the work is a habit or a spike. Same
// encoding as the calendar (one quantity over a two-dimensional time index, so a
// heatmap) at a coarser bin: months, not days, because a decade of days is 3,650
// cells and at README width that is texture rather than data.
//
// The rows are years, which makes each column a season: a reader scanning down
// March sees whether March is always busy or was busy once. That comparison is the
// only reason to draw this instead of six calendars.
//
// Colour reuses the calendar's ramp and the shared quartile bands, so a shade means
// the same thing on both cards — with the bands taken over months here, since a
// monthly total and a daily one are not the same quantity. The year total sits in a
// column of digits at the right, because the one number per row that a reader will
// actually quote should not have to be read off a colour.
func renderYears(s *Stats) string {
	if len(s.Years) == 0 {
		return renderEmpty("~/years", "No multi-year history available yet.")
	}

	const (
		w        = 460.0
		pad      = 18.0
		gutter   = 32.0 // year labels
		cellW    = 26.0
		cellH    = 18.0
		colStep  = cellW + surfaceGap
		rowStep  = cellH + surfaceGap
		totalW   = 48.0
		totalGap = 10.0
		chromeH  = 30.0
	)

	rows := s.Years
	// Six years is the query's span; the cap is here so the card cannot grow past
	// its column if that span ever widens.
	if len(rows) > 6 {
		rows = rows[len(rows)-6:]
	}

	months := make([]int, 0, len(rows)*12)
	best := rows[0]
	for _, y := range rows {
		if y.Total > best.Total {
			best = y
		}
		for _, v := range y.Months {
			if v > 0 {
				months = append(months, v)
			}
		}
	}
	bands := bandsOf(months)

	plotX := pad + gutter
	headY := chromeH + 22.0
	gridTop := headY + 9.0
	gridH := float64(len(rows))*rowStep - surfaceGap
	legendY := gridTop + gridH + 26.0
	h := legendY + 30.0

	peakMonth, peakCount := best.Peak()
	c := newCanvas(w, h, "Contributions by month and year",
		fmt.Sprintf("%s across %s, one cell per month. Busiest year was %d with %s, "+
			"peaking in %s at %s. Shading runs in four bands by monthly count: %s, %s, %s and %s.",
			commas(yearsTotal(rows)), plural(len(rows), "year"), best.Year, commas(best.Total),
			time.Month(peakMonth+1).String(), plural(peakCount, "contribution"),
			bandLabel(1, bands[0]), bandLabel(bands[0]+1, bands[1]),
			bandLabel(bands[1]+1, bands[2]), bandLabel(bands[2]+1, 0)))

	// Additive only: a cell settles at full opacity and opens at .4, so a renderer
	// that never runs the animation shows a dimmer grid rather than an empty one.
	c.style(".yc{animation:yearIn 700ms ease-out}")
	c.style("@keyframes yearIn{from{opacity:.4}to{opacity:1}}")
	for i := range rows {
		c.style(fmt.Sprintf(".yr%d{animation-duration:%dms}", i, 520+i*90))
	}

	c.windowChrome(fmt.Sprintf("~/years · %s over %s",
		commas(yearsTotal(rows)), plural(len(rows), "year")))

	for m := 0; m < 12; m++ {
		c.text(plotX+float64(m)*colStep+cellW/2, headY, time.Month(m + 1).String()[:3],
			textOpts{size: 8.5, fill: inkLow, anchor: "middle"})
	}
	c.text(w-pad, headY, "total", textOpts{size: 8.5, fill: inkLow, anchor: "end"})

	for i, y := range rows {
		rowY := gridTop + float64(i)*rowStep
		c.text(pad, rowY+cellH-5.5, fmt.Sprintf("%d", y.Year),
			textOpts{size: 9.5, fill: inkMid})

		for m, v := range y.Months {
			fill := gridline
			if lvl := heatLevel(v, bands); lvl > 0 {
				fill = heatSteps[lvl-1]
			}
			c.group(fmt.Sprintf("%s %d — %s", time.Month(m+1).String(), y.Year,
				plural(v, "contribution")), fmt.Sprintf(`class="yc yr%d"`, i))
			c.rect(plotX+float64(m)*colStep, rowY, cellW, cellH, fill, 2)
			c.groupEnd()
		}

		// The busiest year is the only row named twice — once in its digits, once in
		// the brighter ink — because "which year was the biggest" is the reading a
		// reader takes away from a grid of six.
		ink := inkMid
		if y.Year == best.Year {
			ink = inkHi
		}
		c.text(w-pad, rowY+cellH-5.5, commas(y.Total),
			textOpts{size: 10, fill: ink, anchor: "end"})
	}

	renderHeatLegend(c, plotX, legendY, "per month", bands)

	// Where the record starts, so the empty months at the top of the first row read
	// as "before this account existed" rather than as quiet ones.
	note := fmt.Sprintf("busiest %d · %s", best.Year, commas(best.Total))
	if !s.CreatedAt.IsZero() {
		note = fmt.Sprintf("account opened %s · %s", s.CreatedAt.Format("Jan 2006"), note)
	}
	caption(c, pad, h-11, w-2*pad, note+" · bands are the quartiles of the active months shown")
	return c.String()
}

func yearsTotal(rows []YearRow) int {
	t := 0
	for _, y := range rows {
		t += y.Total
	}
	return t
}

// BestYear is the heaviest year in the record, for the table view. Zero value and
// false when there is no multi-year record at all.
func (s *Stats) BestYear() (YearRow, bool) {
	if len(s.Years) == 0 {
		return YearRow{}, false
	}
	best := s.Years[0]
	for _, y := range s.Years {
		if y.Total > best.Total {
			best = y
		}
	}
	return best, true
}
