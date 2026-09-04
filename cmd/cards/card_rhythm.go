package main

import (
	"fmt"
	"time"
)

// The rhythm card: when in the week the commits actually happen.
//
// Form. One quantity over two categorical time axes — hour of day against day of
// week — which is a heatmap. Marginals (a 24-bar chart of hours beside a 7-bar
// chart of days) would be easier to read and would answer a different, smaller
// question: they cannot show that the late nights are Sundays. The joint
// distribution is the whole point, so the grid stays.
//
// It shares the calendar card's ramp and its quartile bands, so a shade means the
// same kind of thing on both cards, and it prints its band ranges for the same
// reason: a sequential legend without numbers asks the reader to guess.
//
// Honesty about the sample. This is not every commit ever written — it is the last
// fifty on each repo's default branch, attributed to this account. The card says so
// in its caption and in its description, because "when I work" drawn from a sample
// that skews toward recent months is a claim about recent months.
// Rows run Monday to Sunday: the interesting claim on a card like this is weeknight
// versus weekend, and a Sunday-first grid splits the weekend across the top and
// bottom edges where nobody can see it as one block.
const mondayFirst = "Mon Tue Wed Thu Fri Sat Sun"

func rhythmRow(t time.Time) int { return (int(t.Weekday()) + 6) % 7 }

func weekdayLabel(row int) string { return mondayFirst[row*4 : row*4+3] }

// rhythmSummary is everything the rhythm card claims in words: where the peak is,
// what window the sample covers, and which clock the hours were read in. The table
// view quotes the same struct, so the two cannot drift apart.
type rhythmSummary struct {
	Sample   int
	Peak     int
	PeakRow  int
	PeakHour int
	Oldest   time.Time
	Newest   time.Time
	UTCOnly  int // stamps that came back with no offset at all

	// Weekend and Night are the two readings a 7x24 grid holds but does not
	// state. The grid shows where the mass is; these say how much of it, which is
	// the difference between a reader inferring "works late" and being told.
	Weekend int // commits on Saturday or Sunday
	Night   int // commits at 22:00-05:59, the hours outside any working day
}

// Share renders a count as its percentage of the sample, which is the only form in
// which "38 weekend commits" means anything.
func (r rhythmSummary) Share(n int) string {
	if r.Sample <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", float64(n)/float64(r.Sample)*100)
}

func (r rhythmSummary) PeakDay() string { return weekdayLabel(r.PeakRow) }

// Clock names the timezone the grid was actually built in. GitHub's GitTimestamp
// keeps the offset the commit was written in, but the schema does not guarantee it
// for every field, so the card discloses what it read rather than asserting local
// time it cannot prove.
func (r rhythmSummary) Clock() string {
	switch {
	case r.Sample > 0 && r.UTCOnly == r.Sample:
		if tzFallback != 0 {
			return fmt.Sprintf("UTC+%d, applied because every stamp came back without an offset", tzFallback)
		}
		return "UTC — every stamp in the sample came back without an offset"
	case r.UTCOnly > 0:
		return fmt.Sprintf("each commit's own clock, except %s recorded in UTC", plural(r.UTCOnly, "stamp"))
	}
	return "each commit's own clock, as git recorded it"
}

// RhythmGrid bins the commit sample into day-of-week rows and hour-of-day columns.
func (s *Stats) RhythmGrid() (grid [7][24]int, sum rhythmSummary) {
	if len(s.CommitStamps) == 0 {
		return grid, sum
	}
	sum.Sample = len(s.CommitStamps)
	sum.Oldest, sum.Newest = s.CommitStamps[0].At, s.CommitStamps[0].At
	for _, st := range s.CommitStamps {
		if st.Offset == 0 {
			sum.UTCOnly++
		}
		if st.At.Before(sum.Oldest) {
			sum.Oldest = st.At
		}
		if st.At.After(sum.Newest) {
			sum.Newest = st.At
		}
		t := st.Local()
		r, h := rhythmRow(t), t.Hour()
		grid[r][h]++
		if r >= 5 {
			sum.Weekend++
		}
		if h >= 22 || h < 6 {
			sum.Night++
		}
		if grid[r][h] > sum.Peak {
			sum.PeakRow, sum.PeakHour, sum.Peak = r, h, grid[r][h]
		}
	}
	return grid, sum
}

func renderRhythm(s *Stats) string {
	if len(s.CommitStamps) == 0 {
		return renderEmpty("~/rhythm", "No commit timestamps in the sample yet.")
	}

	const (
		w       = 460.0
		pad     = 18.0
		cell    = 14.0
		step    = cell + surfaceGap
		chromeH = 30.0
	)
	// The gutter absorbs whatever the 24 columns leave over, so the card lands on a
	// round 460 and the weekday labels get the slack instead of a ragged edge.
	gutter := w - 2*pad - (24*step - surfaceGap)

	grid, sum := s.RhythmGrid()

	flat := make([]int, 0, 7*24)
	for r := 0; r < 7; r++ {
		for h := 0; h < 24; h++ {
			if grid[r][h] > 0 {
				flat = append(flat, grid[r][h])
			}
		}
	}
	bands := bandsOf(flat)

	gridTop := chromeH + 24.0
	gridH := 7*step - surfaceGap
	legendY := gridTop + gridH + 26
	// The extra band under the legend carries the weekend and night shares, which
	// are readings the grid holds but never states.
	h := legendY + 52

	peakDay, peakCol, peak := sum.PeakDay(), sum.PeakHour, sum.Peak
	oldest, newest, clock := sum.Oldest, sum.Newest, sum.Clock()

	c := newCanvas(w, h, "Commit rhythm",
		fmt.Sprintf("Hour of day against day of week for a sample of %s, %s to %s. "+
			"Busiest slot is %s at %02d:00 with %s. Read in %s. "+
			"%s of the sample is weekend work and %s of it lands between 22:00 and 06:00. "+
			"Shading runs in four bands by commits per slot: %s, %s, %s and %s.",
			plural(len(s.CommitStamps), "commit"),
			oldest.Format("2 Jan 2006"), newest.Format("2 Jan 2006"),
			peakDay, peakCol, plural(peak, "commit"), clock,
			sum.Share(sum.Weekend), sum.Share(sum.Night),
			bandLabel(1, bands[0]), bandLabel(bands[0]+1, bands[1]),
			bandLabel(bands[1]+1, bands[2]), bandLabel(bands[2]+1, 0)))

	// Additive only, and one duration per column so the sweep runs left to right
	// across the day without a delay that could hide a cell.
	c.style(".rc{animation:heat 800ms ease-out}")
	c.style("@keyframes heat{from{opacity:.35}to{opacity:1}}")
	for i := 0; i < 24; i++ {
		c.style(fmt.Sprintf(".rh%d{animation-duration:%dms}", i, 560+i*26))
	}

	c.windowChrome(fmt.Sprintf("~/rhythm  ·  %s sampled", plural(len(s.CommitStamps), "commit")))

	// Every third hour, so the axis is readable without stacking labels.
	for hr := 0; hr < 24; hr += 3 {
		c.text(pad+gutter+float64(hr)*step, gridTop-8, fmt.Sprintf("%02d", hr),
			textOpts{size: 8.5, fill: inkLow})
	}
	for r := 0; r < 7; r++ {
		c.text(pad+gutter-6, gridTop+float64(r)*step+cell-3.5, weekdayLabel(r),
			textOpts{size: 8.5, fill: inkLow, anchor: "end"})
	}

	for r := 0; r < 7; r++ {
		for hr := 0; hr < 24; hr++ {
			count := grid[r][hr]
			fill := gridline
			if lvl := heatLevel(count, bands); lvl > 0 {
				fill = heatSteps[lvl-1]
			}
			c.group(fmt.Sprintf("%s %02d:00–%02d:59 — %s", weekdayLabel(r), hr, hr,
				plural(count, "commit")), fmt.Sprintf(`class="rc rh%d"`, hr))
			c.rect(pad+gutter+float64(hr)*step, gridTop+float64(r)*step, cell, cell, fill, 2)
			c.groupEnd()
		}
	}

	renderHeatLegend(c, pad+gutter, legendY, "per slot", bands)
	c.text(w-pad, legendY, fmt.Sprintf("busiest %s %02d:00", peakDay, peakCol),
		textOpts{size: 9.5, fill: inkLow, anchor: "end"})
	// Text wears ink tokens: the figures sit in primary ink and the words that
	// qualify them in muted, so nothing here competes with the grid for colour.
	c.text(pad, legendY+20, fmt.Sprintf("weekends %s  ·  nights 22:00–06:00 %s  ·  %s",
		sum.Share(sum.Weekend), sum.Share(sum.Night), plural(sum.Sample, "commit")),
		textOpts{size: 9.5, fill: inkMid})

	caption(c, pad, h-22, w-2*pad, fmt.Sprintf("sample: last 50 commits per repo, %s to %s",
		oldest.Format("2 Jan 2006"), newest.Format("2 Jan 2006")))
	caption(c, pad, h-11, w-2*pad, "clock: "+clock)
	return c.String()
}
