package main

import (
	"fmt"
	"math"
)

// The drift card: what the work is turning into.
//
// Every other card on this profile is a level — how much, how many, how long. This
// one is the only derivative, and it exists because the interesting thing about a
// language mix is not its shape today but which way it is moving. A reader can
// infer "mostly Python" from the languages card; nobody can infer "Go is up nine
// points since spring" from anything but a record kept on purpose.
//
// Form. Two values per category at two points in time is a dumbbell: a dot for
// then, a dot for now, a connector carrying the direction. A pair of grouped bars
// would need twice the ink to say the same thing, and a slope chart with seven
// crossing lines is unreadable at 460px. Direction lands three ways over — the
// geometry of the connector, the signed number at the right, and the arrow glyph —
// so it survives greyscale, a colourblind reader, and a screen reader in turn.
//
// One axis: share in percentage points, zero to the largest share in either
// reading. Points, not percent-of-percent, because 40 -> 44 is four points, and
// billing that as "10% growth" is a different and much weaker claim.
func renderDrift(s *Stats) string {
	moves, age, ok := s.LanguageDrift(30)
	if !ok {
		// Day one is the honest empty state: the archive cannot show a direction it
		// has not had time to measure. Saying so is better than drawing a flat card
		// that looks like a finding.
		return renderEmpty("~/drift", "The archive starts today. Drift appears once there are two readings.")
	}

	const (
		w        = 460.0
		pad      = 18.0
		labelW   = 104.0
		deltaW   = 62.0
		rowPitch = 26.0
		chromeH  = 30.0
		dotR     = 4.5
		nameSize = 10.5
	)

	// Cap the rows so a profile with a long tail of one-off languages does not grow
	// the card past the column it is rendered in. LanguageDrift ranks by the size of
	// the move, so the cap keeps the biggest movers rather than an arbitrary eight —
	// and the table view carries the whole list either way.
	const maxRows = 8
	shown, hidden := moves, 0
	if len(shown) > maxRows {
		shown, hidden = shown[:maxRows], len(shown)-maxRows
	}
	moves = shown

	top := 0.0
	for _, m := range moves {
		top = math.Max(top, math.Max(m.Then, m.Now))
	}
	if top <= 0 {
		top = 1
	}

	plotX := pad + labelW
	plotW := w - pad - deltaW - plotX
	headY := chromeH + 22.0
	plotTop := headY + 20.0
	h := math.Ceil(plotTop + float64(len(moves))*rowPitch + 34)

	biggest := moves[0] // already the largest move: LanguageDrift sorts by it

	c := newCanvas(w, h, "Language drift",
		fmt.Sprintf("The %s that moved most as a share of tracked bytes over the last %d days, "+
			"in percentage points, biggest move first. Biggest mover is %s at %s. Each row shows "+
			"the share then as a hollow dot and now as a filled one.",
			plural(len(moves), "language"), age, biggest.Name, points(biggest.Delta())))

	// Additive only: the connector grows from its "then" end, and both dots are at
	// full opacity in the settled frame.
	c.style(".dr{animation:driftIn 700ms ease-out}")
	c.style("@keyframes driftIn{from{opacity:.35}to{opacity:1}}")

	c.windowChrome(fmt.Sprintf("~/drift · share in points, last %d days", age))

	c.text(pad, headY, "language", textOpts{size: 9, fill: inkLow})
	c.text(plotX, headY, fmt.Sprintf("share then → now  (0–%.0f pt)", math.Ceil(top)),
		textOpts{size: 9, fill: inkLow})
	c.text(w-pad, headY, "move", textOpts{size: 9, fill: inkLow, anchor: "end"})

	scale := plotW / top
	for i, m := range moves {
		y := plotTop + float64(i)*rowPitch + rowPitch/2
		c.text(pad, y+3.5, truncateToWidth(m.Name, nameSize, labelW-6),
			textOpts{size: nameSize, fill: inkHi})

		thenX := plotX + m.Then*scale
		nowX := plotX + m.Now*scale

		// The track is the full axis, so a small share reads as small rather than
		// as simply short.
		c.rect(plotX, y-1, plotW, 2, surfaceUp, 1)

		// The connector is the change itself: its length is the magnitude and its
		// direction is the sign, before any number is read.
		lo, hi := math.Min(thenX, nowX), math.Max(thenX, nowX)
		if hi-lo > 1 {
			c.rect(lo, y-1.5, hi-lo, 3, accentDim, 1.5, `class="dr"`)
		}

		c.group(fmt.Sprintf("%s: %.1f%% then, %.1f%% now (%s over %dd)",
			m.Name, m.Then, m.Now, points(m.Delta()), age))
		// Hollow for the old reading, filled for the current one: two states of one
		// mark, so which end is "now" never depends on colour.
		c.dot(thenX, y, dotR, surface)
		c.dot(thenX, y, dotR, "none", `stroke="`+inkMid+`"`, `stroke-width="1.5"`)
		c.dot(nowX, y, dotR, accent, `stroke="`+surface+`"`, `stroke-width="2"`, `class="dr"`)
		c.groupEnd()

		c.text(w-pad, y+3.5, points(m.Delta()),
			textOpts{size: 10, fill: moveInk(m.Delta()), anchor: "end"})
	}

	// The caption has one line, so the tail displaces the gloss rather than
	// extending past the card and getting cut.
	tail := "points of all tracked bytes"
	if hidden > 0 {
		tail = fmt.Sprintf("%d more in the table below", hidden)
	}
	caption(c, pad, h-11, w-2*pad,
		fmt.Sprintf("hollow = %d days ago  ·  filled = today  ·  %s", age, tail))
	return c.String()
}

// points renders a change in percentage points with its direction as a glyph, so
// the sign survives a greyscale render and a screen reader alike.
func points(d float64) string {
	switch {
	case d >= 0.05:
		return fmt.Sprintf("▲ %.1f pt", d)
	case d <= -0.05:
		return fmt.Sprintf("▼ %.1f pt", -d)
	}
	return "flat"
}

// moveInk keeps text in ink tokens rather than the series colour: only the mark
// carries accent, and a rising figure gets the brighter of two neutral inks.
func moveInk(d float64) string {
	if math.Abs(d) >= 0.05 {
		return inkHi
	}
	return inkLow
}
