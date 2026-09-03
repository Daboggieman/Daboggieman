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

	// Every animation here is additive: the settled frame is the base style, and
	// the keyframes only add motion on top of it. A staggered reveal that starts
	// from opacity:0 renders the whole card blank in any context where the
	// animation does not advance, which is not a trade worth making on a hero card.
	//
	// The meter fades up rather than growing from scaleX(0) for a sharper reason
	// than looks: a renderer that samples the first frame of a running animation
	// would catch the fill at zero width, and a meter that reads 0/25 when the
	// real value is 12/25 is a wrong number, not a missing flourish. Opacity can
	// only ever cost the animation, never the value.
	c.style(".cur{animation:blink 1.1s steps(1,end) infinite}")
	c.style("@keyframes blink{0%,49%{opacity:1}50%,100%{opacity:0}}")
	c.style(".grow{animation:lampUp 900ms ease-out}")
	c.style("@keyframes lampUp{from{opacity:.35}to{opacity:1}}")

	c.windowChrome("raphel@github: ~/profile — zsh")

	for i, l := range lines {
		y := y0 + float64(i)*lineH
		x := pad
		if l.prompt {
			c.text(x, y, sigil, textOpts{size: size, fill: accent, weight: "500"})
			x += sigilW
		}
		fill := l.fill
		if fill == "" {
			fill = inkHi
		}
		c.text(x, y, l.text, textOpts{size: size, fill: fill})
	}

	renderStreakMeter(c, s, pad, meterY-6, w-2*pad)

	// A prompt waiting on input, with the one piece of looping motion on the card.
	cursorY := meterY + 30
	c.text(pad, cursorY, sigil, textOpts{size: size, fill: accent, weight: "500"})
	c.rect(pad+sigilW, cursorY-10, size*charAdvance, 13, accent, 1, `class="cur"`)

	c.text(pad, h-16, fmt.Sprintf("rendered by ./cmd/cards · %s",
		s.GeneratedAt.Format("2006-01-02")), textOpts{size: 9.5, fill: inkLow})
	return c.String()
}

// renderStreakMeter draws the current streak as a ratio against the personal
// best: one value against a limit, which is a meter rather than a chart. The
// unfilled track is a dimmer step of the fill's own ramp, so the state reads
// across the whole bar instead of only where the fill ends.
func renderStreakMeter(c *canvas, s *Stats, x, y, w float64) {
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

	c.path(hBarPath(x, y, trackW, trackH, barRadius), trackGreen)
	// The fill's settled width is its real value; the animation only grows into it.
	fillW := ratio * trackW
	c.hBar(x, y, fillW, trackH, accent,
		fmt.Sprintf("current streak %d days; personal best %d days", s.CurrentStreak, s.LongestStreak),
		`class="grow"`)
	// Two touching fills get a gap in the surface colour, not a stroke. Fill and
	// track are two steps of one ramp, which is a clear step at full size but a
	// soft one at the scale a README renders at; the notch makes where the value
	// ends unmistakable instead of merely legible.
	if fillW > 0 && fillW < trackW-surfaceGap {
		c.rect(x+fillW, y, surfaceGap, trackH, surface, 0)
	}
	c.text(x+trackW+labelGap, y+trackH-1, label, textOpts{size: 11, fill: inkMid})
}
