package main

import "fmt"

// renderLanguages draws a horizontal bar chart ranking languages by share of
// bytes across every owned non-fork repo the token could see — which, with a
// repo-scoped PAT, includes private ones. The description says so rather than
// claiming "public repositories": the token decides what got counted, and a card
// that overstates its own scope is a wrong number wearing a caption.
//
// Form: magnitude comparison, so bars scaled to the leader and one hue for every
// bar. Languages are nominal categories, so a value-ramp across them would
// double-encode length as hue; only the folded "Other" bucket differs, wearing
// the de-emphasis gray. Every bar is directly labelled, so identity and value
// never depend on color.
func renderLanguages(s *Stats) string {
	langs := s.Languages
	if len(langs) == 0 {
		return renderEmpty("~/languages", "No language data available yet.")
	}

	const (
		w         = 460.0
		pad       = 18.0
		labelW    = 104.0
		valueW    = 54.0
		rowPitch  = 24.0
		barThick  = 10.0
		nameSize  = 11.0
		valueSize = 11.0
	)

	rows := float64(len(langs))
	chromeH := 30.0
	plotTop := chromeH + 18.0
	h := plotTop + rows*rowPitch + 34.0

	scope := fmt.Sprintf("%d repositories", s.RepoCount)
	if s.RepoCount == 1 {
		scope = "1 repository"
	}
	if n := s.PrivateCount(); n > 0 {
		scope += fmt.Sprintf(", %d of them private", n)
	}
	c := newCanvas(w, h, "Language mix",
		fmt.Sprintf("Share of code by bytes across %s, ranked. Largest is %s at %.1f%%.",
			scope, langs[0].Name, langs[0].Share*100))
	c.windowChrome(fmt.Sprintf("~/languages  ·  %d repos", s.RepoCount))

	axisX := pad + labelW
	trackW := w - pad - valueW - 10 - axisX
	top := langs[0].Share
	if top <= 0 {
		top = 1
	}

	// Solid hairline axis, one step off the surface. Never dashed.
	c.raw(`  <line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"/>`,
		n(axisX-6), n(plotTop-6), n(axisX-6), n(plotTop+rows*rowPitch-rowPitch+barThick+6), baseline, n(hairline))

	for i, l := range langs {
		y := plotTop + float64(i)*rowPitch
		barW := (l.Share / top) * trackW
		if barW < 2 {
			barW = 2
		}

		fill := accent
		if l.Name == "Other" {
			fill = deEmph
		}

		pct := fmt.Sprintf("%.1f%%", l.Share*100)
		tip := fmt.Sprintf("%s — %s (%s)", l.Name, pct, humanBytes(l.Bytes))
		c.text(pad, y+barThick-1, truncateToWidth(l.Name, nameSize, labelW-10),
			textOpts{size: nameSize, fill: inkMid, tooltip: tip})
		c.hBar(axisX, y, barW, barThick, fill, tip)
		// The value sits outside the data-end in a reserved column, so it is
		// never clipped by a short bar.
		c.text(w-pad, y+barThick-1, pct, textOpts{
			size: valueSize, fill: inkHi, weight: "500", anchor: "end",
		})
	}

	c.text(pad, h-14, "% of code by bytes  ·  bars scaled to the leader", textOpts{size: 10, fill: inkLow})
	return c.String()
}

// renderEmpty produces a card that says so, rather than an empty frame, when
// the API returns nothing to plot.
func renderEmpty(title, msg string) string {
	c := newCanvas(460, 110, title, msg)
	c.windowChrome(title)
	c.text(18, 70, msg, textOpts{size: 12, fill: inkMid})
	return c.String()
}

func humanBytes(b int) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f kB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
