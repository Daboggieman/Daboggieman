package main

import "fmt"

// The collaboration card: the part of a year's work that involved somebody else.
//
// Commits are deliberately not one of the bars. A year here is roughly 1,200
// commits against a few dozen pull requests, so on a shared axis the three
// collaborative measures would be slivers a pixel or two long and the chart would
// have answered a question nobody asked. One axis is not negotiable, so the fix is
// to chart only measures of the same order and let commits be the denominator the
// hero figure is quoted against.
//
// Form: three nominal categories ranked by one quantity, so horizontal bars scaled
// to the leader, sorted, every bar directly labelled. One hue across all three,
// because the categories are names and not a scale. The movement line under each
// label comes out of the archive (see history.go) and is empty until the archive is
// old enough to support it.
func renderCollab(s *Stats) string {
	acts := s.PullRequests + s.Reviews + s.Issues
	if acts == 0 && s.Commits == 0 {
		return renderEmpty("~/collab", "No contribution activity in the last 365 days.")
	}

	const (
		w         = 460.0
		pad       = 18.0
		labelW    = 112.0
		valueW    = 44.0
		rowPitch  = 32.0
		barThick  = 10.0
		chromeH   = 30.0
		nameSize  = 11.0
		deltaSize = 8.5
	)

	rows := []struct {
		name  string
		value int
		delta string
	}{
		{"pull requests", s.PullRequests,
			s.trendFor(s.PullRequests, func(p Snapshot) int { return p.PullRequests })},
		{"reviews", s.Reviews,
			s.trendFor(s.Reviews, func(p Snapshot) int { return p.Reviews })},
		{"issues", s.Issues,
			s.trendFor(s.Issues, func(p Snapshot) int { return p.Issues })},
	}
	// Ranked, tallest first, by the same rule the skyline uses.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].value > rows[j-1].value; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}

	heroLabelY := chromeH + 22.0
	heroValueY := heroLabelY + 34.0
	plotTop := heroValueY + 30.0
	h := plotTop + float64(len(rows))*rowPitch + 32.0

	share := 0.0
	if s.TotalContributions > 0 {
		share = float64(acts) / float64(s.TotalContributions) * 100
	}

	c := newCanvas(w, h, "Collaboration",
		fmt.Sprintf("%s collaborative acts in the last 365 days — %s opened, %s reviewed, %s filed. "+
			"That is %.1f%% of %s total contributions; the rest is commits.",
			commas(acts), plural(s.PullRequests, "pull request"), plural(s.Reviews, "review"),
			plural(s.Issues, "issue"), share, commas(s.TotalContributions)))

	// Additive only: the bars settle at their real width and the animation fades
	// them in, so a frozen first frame is a dim chart rather than an empty one.
	c.style(".cb{animation:fadeBar 700ms ease-out}")
	c.style("@keyframes fadeBar{from{opacity:.35}to{opacity:1}}")
	for i := range rows {
		c.style(fmt.Sprintf(".cb%d{animation-duration:%dms}", i, 560+i*130))
	}

	c.windowChrome("~/collab  ·  last 365 days")

	c.text(pad, heroLabelY, "collaborative acts", textOpts{size: 10, fill: inkLow})
	hero := compact(acts)
	c.text(pad, heroValueY, hero, textOpts{
		size: 34, fill: inkHi, weight: "700",
		tooltip: fmt.Sprintf("%s pull requests, reviews and issues in the last 365 days", commas(acts)),
	})
	c.text(pad+textWidth(hero, 34)+12, heroValueY-11,
		fmt.Sprintf("%.1f%% of %s contributions", share, compact(s.TotalContributions)),
		textOpts{size: 10, fill: inkMid})
	c.text(pad+textWidth(hero, 34)+12, heroValueY-1,
		fmt.Sprintf("alongside %s commits", commas(s.Commits)),
		textOpts{size: 10, fill: inkLow})
	if d := s.trendFor(acts, func(p Snapshot) int { return p.PullRequests + p.Reviews + p.Issues }); d != "" {
		c.text(pad+textWidth(hero, 34)+12, heroValueY+9, d, textOpts{size: 10, fill: inkLow})
	}

	c.hRule(pad, w-pad, plotTop-16, gridline)

	axisX := pad + labelW
	trackW := w - pad - valueW - 10 - axisX
	peak := float64(rows[0].value)
	if peak <= 0 {
		peak = 1
	}

	// Solid hairline axis, one step off the surface. Never dashed.
	c.raw(`  <line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"/>`,
		n(axisX-6), n(plotTop-6), n(axisX-6), n(plotTop+float64(len(rows))*rowPitch-rowPitch+barThick+6),
		baseline, n(hairline))

	for i, r := range rows {
		y := plotTop + float64(i)*rowPitch
		tip := fmt.Sprintf("%s — %s in the last 365 days", r.name, commas(r.value))
		if r.delta != "" {
			tip += " (" + r.delta + ")"
		}

		c.text(pad, y+barThick-1, truncateToWidth(r.name, nameSize, labelW-8),
			textOpts{size: nameSize, fill: inkHi, tooltip: tip})
		if r.delta != "" {
			c.text(pad, y+barThick+11, truncateToWidth(r.delta, deltaSize, labelW-8),
				textOpts{size: deltaSize, fill: inkLow})
		}

		// The empty track shows the distance to the leader, so a short bar reads as
		// "less than" rather than as "the axis stops here".
		c.path(hBarPath(axisX, y, trackW, barThick, barRadius), surfaceUp)
		bw := float64(r.value) / peak * trackW
		if bw > 0 && bw < barRadius*2 {
			bw = barRadius * 2 // a value of one is still a visible mark
		}
		c.hBar(axisX, y, bw, barThick, accent, tip, fmt.Sprintf(`class="cb cb%d"`, i))

		c.text(w-pad, y+barThick-1, commas(r.value),
			textOpts{size: 11, fill: inkHi, weight: "500", anchor: "end", tooltip: tip})
	}

	caption(c, pad, h-12, w-2*pad, "commits are the denominator, not a bar: they would dwarf all three")
	return c.String()
}
