package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// n formats a coordinate compactly: no trailing zeros, no scientific notation.
func n(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func joinAttrs(attrs []string) string {
	if len(attrs) == 0 {
		return ""
	}
	return " " + strings.Join(attrs, " ")
}

// esc escapes text for XML content and attribute values.
func esc(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// hBarPath builds a horizontal bar growing rightward from a baseline at x:
// square where it meets the baseline, rounded at the data-end.
func hBarPath(x, y, w, h, r float64) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	if r > w {
		r = w
	}
	if r > h/2 {
		r = h / 2
	}
	var b strings.Builder
	fmt.Fprintf(&b, "M%s %s", n(x), n(y))
	fmt.Fprintf(&b, "H%s", n(x+w-r))
	fmt.Fprintf(&b, "A%s %s 0 0 1 %s %s", n(r), n(r), n(x+w), n(y+r))
	fmt.Fprintf(&b, "V%s", n(y+h-r))
	fmt.Fprintf(&b, "A%s %s 0 0 1 %s %s", n(r), n(r), n(x+w-r), n(y+h))
	fmt.Fprintf(&b, "H%sZ", n(x))
	return b.String()
}

// vBarPath builds a column growing upward from a baseline at y+h: square at the
// baseline, rounded on the cap.
func vBarPath(x, y, w, h, r float64) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	if r > h {
		r = h
	}
	if r > w/2 {
		r = w / 2
	}
	var b strings.Builder
	fmt.Fprintf(&b, "M%s %s", n(x), n(y+h))
	fmt.Fprintf(&b, "V%s", n(y+r))
	fmt.Fprintf(&b, "A%s %s 0 0 1 %s %s", n(r), n(r), n(x+r), n(y))
	fmt.Fprintf(&b, "H%s", n(x+w-r))
	fmt.Fprintf(&b, "A%s %s 0 0 1 %s %s", n(r), n(r), n(x+w), n(y+r))
	fmt.Fprintf(&b, "V%sZ", n(y+h))
	return b.String()
}

func (c *canvas) path(d, fill string, attrs ...string) {
	if d == "" {
		return
	}
	c.raw(`  <path d="%s" fill="%s"%s/>`, d, fill, joinAttrs(attrs))
}

// hBar draws a horizontal bar with an optional native tooltip.
func (c *canvas) hBar(x, y, w, h float64, fill, tooltip string, attrs ...string) {
	d := hBarPath(x, y, w, h, barRadius)
	if d == "" {
		return
	}
	if tooltip == "" {
		c.path(d, fill, attrs...)
		return
	}
	c.raw(`  <path d="%s" fill="%s"%s><title>%s</title></path>`, d, fill, joinAttrs(attrs), esc(tooltip))
}

// vBar draws a column with an optional native tooltip.
func (c *canvas) vBar(x, y, w, h float64, fill, tooltip string, attrs ...string) {
	d := vBarPath(x, y, w, h, barRadius)
	if d == "" {
		return
	}
	if tooltip == "" {
		c.path(d, fill, attrs...)
		return
	}
	c.raw(`  <path d="%s" fill="%s"%s><title>%s</title></path>`, d, fill, joinAttrs(attrs), esc(tooltip))
}

// windowChrome draws the card surface plus a title bar with three dots, so each
// card reads as a terminal window.
func (c *canvas) windowChrome(title string) float64 {
	const barH = 30.0
	c.rect(0, 0, c.w, c.h, surface, 8)
	c.raw(`  <path d="M0 8a8 8 0 0 1 8-8h%sa8 8 0 0 1 8 8v%sH0Z" fill="%s"/>`,
		n(c.w-16), n(barH-8), surfaceUp)
	c.hRule(0, c.w, barH, gridline)
	for i, col := range []string{"#FF5F57", "#FEBC2E", "#28C840"} {
		c.raw(`  <circle cx="%s" cy="15" r="4" fill="%s" opacity="0.85"/>`, n(16+float64(i)*16), col)
	}
	c.text(72, 19.5, title, textOpts{size: 11, fill: inkLow})
	return barH
}

// pt is a point in card space.
type pt struct{ x, y float64 }

// poly draws a filled polygon. Used for the axonometric faces on the city card,
// where a face is one plane of a single mark rather than a mark of its own.
func (c *canvas) poly(fill string, pts []pt, attrs ...string) {
	if len(pts) < 3 {
		return
	}
	var b strings.Builder
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(n(p.x))
		b.WriteByte(',')
		b.WriteString(n(p.y))
	}
	c.raw(`  <polygon points="%s" fill="%s"%s/>`, b.String(), fill, joinAttrs(attrs))
}

// group opens a <g>, optionally carrying a native tooltip that covers every
// child, so hovering anywhere on a composite mark names the whole thing.
func (c *canvas) group(tooltip string, attrs ...string) {
	c.raw(`  <g%s>`, joinAttrs(attrs))
	if tooltip != "" {
		c.raw(`    <title>%s</title>`, esc(tooltip))
	}
}

func (c *canvas) groupEnd() { c.raw(`  </g>`) }

// star draws a five-pointed star centred on (cx, cy) with the given outer radius.
//
// It is a path rather than the ★ character on purpose: U+2605 is outside the
// coverage of every font in monoStack, so a text star renders as whatever the
// viewer's fallback chain substitutes — or as tofu where nothing matches. The card
// is served as an image, so that resolution happens on a machine this generator
// never sees. A path always draws.
func (c *canvas) star(cx, cy, r float64, fill string, attrs ...string) {
	// A 5/2 star polygon: alternate outer and inner vertices every 36°, starting
	// at the top point. .382 is the inner/outer ratio that gives the regular star.
	pts := make([]pt, 0, 10)
	for i := 0; i < 10; i++ {
		rad := r
		if i%2 == 1 {
			rad = r * 0.382
		}
		a := -math.Pi/2 + float64(i)*math.Pi/5
		pts = append(pts, pt{cx + rad*math.Cos(a), cy + rad*math.Sin(a)})
	}
	c.poly(fill, pts, attrs...)
}

// dot draws a data point. Markers are 8px or larger by the mark spec, which is
// what makes a dumbbell's two ends distinguishable at README scale.
func (c *canvas) dot(cx, cy, r float64, fill string, attrs ...string) {
	c.raw(`  <circle cx="%s" cy="%s" r="%s" fill="%s"%s/>`,
		n(cx), n(cy), n(r), fill, joinAttrs(attrs))
}
