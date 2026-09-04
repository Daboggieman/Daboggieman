package main

import (
	"fmt"
	"strings"
)

// canvas accumulates SVG markup. Cards are static: no <script>, because GitHub
// strips it, and no external references, because camo will not follow them.
// CSS @keyframes do survive, which is the only animation channel available.
type canvas struct {
	b      strings.Builder
	w, h   float64
	title  string
	desc   string
	styles []string
}

func newCanvas(w, h float64, title, desc string) *canvas {
	return &canvas{w: w, h: h, title: title, desc: desc}
}

// style registers a CSS rule emitted inside the card's <style> block.
func (c *canvas) style(rule string) { c.styles = append(c.styles, rule) }

func (c *canvas) raw(format string, args ...any) {
	fmt.Fprintf(&c.b, format, args...)
	c.b.WriteByte('\n')
}

func (c *canvas) rect(x, y, w, h float64, fill string, rx float64, attrs ...string) {
	if w <= 0 || h <= 0 {
		return
	}
	c.raw(`  <rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="%s"%s/>`,
		n(x), n(y), n(w), n(h), n(rx), fill, joinAttrs(attrs))
}

// hRule draws a solid hairline. Gridlines are never dashed.
func (c *canvas) hRule(x1, x2, y float64, stroke string) {
	c.raw(`  <line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"/>`,
		n(x1), n(y), n(x2), n(y), stroke, n(hairline))
}

type textOpts struct {
	size    float64
	fill    string
	weight  string // "400", "500", "700"
	anchor  string // "", "middle", "end"
	opacity string
	attrs   []string
	tooltip string // native <title> tooltip; SVG needs no JS for this
}

func (c *canvas) text(x, y float64, s string, o textOpts) {
	if o.size == 0 {
		o.size = 12
	}
	if o.fill == "" {
		o.fill = inkMid
	}
	if o.weight == "" {
		o.weight = "400"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, `  <text x="%s" y="%s" font-family="%s" font-size="%s" font-weight="%s" fill="%s"`,
		n(x), n(y), monoStack, n(o.size), o.weight, o.fill)
	if o.anchor != "" {
		fmt.Fprintf(&sb, ` text-anchor="%s"`, o.anchor)
	}
	if o.opacity != "" {
		fmt.Fprintf(&sb, ` opacity="%s"`, o.opacity)
	}
	sb.WriteString(joinAttrs(o.attrs))
	sb.WriteString(">")
	if o.tooltip != "" {
		fmt.Fprintf(&sb, "<title>%s</title>", esc(o.tooltip))
	}
	sb.WriteString(esc(s))
	sb.WriteString("</text>")
	c.raw("%s", sb.String())
}

func (c *canvas) String() string {
	var out strings.Builder
	fmt.Fprintf(&out,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" role="img" aria-labelledby="t d">`,
		n(c.w), n(c.h), n(c.w), n(c.h))
	out.WriteByte('\n')
	fmt.Fprintf(&out, "  <title id=\"t\">%s</title>\n  <desc id=\"d\">%s</desc>\n", esc(c.title), esc(c.desc))
	if len(c.styles) > 0 {
		out.WriteString("  <style>\n")
		// Readers who ask for reduced motion get the finished frame, not the animation.
		out.WriteString("    @media (prefers-reduced-motion: reduce){*{animation:none!important;opacity:1!important}}\n")
		for _, s := range c.styles {
			fmt.Fprintf(&out, "    %s\n", s)
		}
		out.WriteString("  </style>\n")
	}
	out.WriteString(c.b.String())
	out.WriteString("</svg>\n")
	return out.String()
}

// caption writes a card's footnote and refuses to let it run past the card's edge.
// An overflowing caption is invisible in a standalone SVG — the viewport simply
// stops — so the failure looks like a sentence that just ends. Clipping it to a
// visible ellipsis turns a silent truncation into an obvious one, and any caption
// that ellipsises is a caption to rewrite.
func caption(c *canvas, x, y, maxW float64, s string) {
	c.text(x, y, truncateToWidth(s, 9, maxW), textOpts{size: 9, fill: inkLow})
}
