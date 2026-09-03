package main

import (
	"fmt"
	"strings"
)

// termLine is one row of the fake shell session.
type termLine struct {
	prompt bool // render the "raphel@github:~$" sigil before the text
	text   string
	fill   string
}

// renderTerminal draws the hero card: a shell session whose output is filled in
// from live profile data, plus a meter comparing the current streak to the
// personal best.
//
// Animation is CSS keyframes only. GitHub strips <script> from SVG and camo will
// not follow external references, so keyframes are the one channel available.
// The reveal is one-shot rather than looping, and prefers-reduced-motion drops
// straight to the settled frame.
func renderTerminal(s *Stats) string {
	const (
		w      = 880.0
		pad    = 20.0
		y0     = 58.0
		lineH  = 21.0
		size   = 13.0
		sigil  = "raphel@github:~$ "
		sigilW = 17 * size * charAdvance
	)

	name := s.Name
	if name == "" {
		name = s.Login
	}
	where := s.Location
	if where == "" {
		where = "the internet"
	}

	stack := strings.ToLower(strings.Join(s.TopLanguageNames(5), " · "))
	if stack == "" {
		stack = "go · python · typescript"
	}

	identity := fmt.Sprintf("%s — %s · building Kairo", name, where)
	if r, ok := s.TopRepo(s.Login); ok {
		identity = fmt.Sprintf("%s — %s · last push: %s", name, where, r.Name)
	}

	lines := []termLine{
		{prompt: true, text: "whoami"},
		{text: identity, fill: inkHi},
		{prompt: true, text: "cat stack.txt"},
		{text: stack, fill: inkHi},
		{prompt: true, text: "git shortlog -sn --all | wc -l"},
		{text: fmt.Sprintf("%s commits · %s repos · %s stars · %s followers",
			compact(s.Commits), compact(s.PublicRepos), compact(s.Stars), compact(s.Followers)),
			fill: inkHi},
		{prompt: true, text: "./streak --meter"},
	}

	meterY := y0 + float64(len(lines))*lineH
	h := meterY + 62.0

	c := newCanvas(w, h, "Raph'el Ogah — profile terminal",
		fmt.Sprintf("%s, based in %s. Works in %s. %s commits, %s repos, %s stars. Current streak %d days against a personal best of %d.",
			name, where, stack, compact(s.Commits), compact(s.PublicRepos), compact(s.Stars),
			s.CurrentStreak, s.LongestStreak))

	c.style(".ln{opacity:0;animation:reveal 620ms ease-out both}")
	c.style("@keyframes reveal{0%{opacity:0;clip-path:inset(0 100% 0 0)}" +
		"6%{opacity:1;clip-path:inset(0 100% 0 0)}100%{opacity:1;clip-path:inset(0 0 0 0)}}")
	c.style(".cur{animation:blink 1.1s steps(1,end) infinite}")
	c.style("@keyframes blink{0%,49%{opacity:1}50%,100%{opacity:0}}")

	c.windowChrome("raphel@github: ~/profile — zsh")

	for i, l := range lines {
		y := y0 + float64(i)*lineH
		delay := fmt.Sprintf(`style="animation-delay:%dms"`, 120+i*260)
		x := pad
		if l.prompt {
			c.text(x, y, sigil, textOpts{
				size: size, fill: accent, weight: "500",
				attrs: []string{`class="ln"`, delay},
			})
			x += sigilW
		}
		fill := l.fill
		if fill == "" {
			fill = inkHi
		}
		c.text(x, y, l.text, textOpts{
			size: size, fill: fill,
			attrs: []string{`class="ln"`, delay},
		})
	}

	renderStreakMeter(c, s, pad, meterY-6, w-2*pad, len(lines))
	c.text(pad, h-16, fmt.Sprintf("rendered by ./cmd/cards · %s",
		s.GeneratedAt.Format("2006-01-02")), textOpts{size: 9.5, fill: inkLow})
	return c.String()
}

// renderStreakMeter draws the current streak as a ratio against the personal
// best: one value against a limit, which is a meter rather than a chart. The
// unfilled track is a dimmer step of the fill's own ramp, so the state reads
// across the whole bar instead of only where the fill ends.
func renderStreakMeter(c *canvas, s *Stats, x, y, w float64, lineCount int) {
	const (
		trackH   = 8.0
		labelGap = 14.0
	)

	label := fmt.Sprintf("%s / %s days  ·  current vs personal best",
		compact(s.CurrentStreak), compact(s.LongestStreak))
	labelW := textWidth(label, 11) + labelGap
	trackW := w - labelW
	if trackW < 120 {
		trackW = 120
	}

	limit := s.LongestStreak
	if limit <= 0 {
		limit = 1
	}
	ratio := float64(s.CurrentStreak) / float64(limit)
	if ratio > 1 {
		ratio = 1
	}

	delay := fmt.Sprintf(`style="animation-delay:%dms"`, 120+lineCount*260)
	c.raw(`  <g class="ln" %s>`, delay)
	c.path(hBarPath(x, y, trackW, trackH, barRadius), trackGreen)
	c.hBar(x, y, ratio*trackW, trackH, accent,
		fmt.Sprintf("current streak %d days; personal best %d days", s.CurrentStreak, s.LongestStreak))
	c.text(x+trackW+labelGap, y+trackH-1, label, textOpts{size: 11, fill: inkMid})
	c.raw(`  </g>`)
}
