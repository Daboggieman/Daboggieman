package main

// Terminal theme tokens.
//
// Every card renders its own dark surface, so a single set of tokens works on
// both GitHub light and dark themes without a media-query flip.
//
// The green steps below were validated as an ordinal ramp against the surface
// with the dataviz palette validator (monotone lightness, adjacent dL >= 0.06,
// dark end 3.73:1 on surface, hue spread 3 deg). They are kept for ordered
// scales only. Language bars are nominal categories, so they all wear accent:
// a value-ramp there would double-encode bar length as hue.
const (
	surface   = "#0D1117" // chart surface (GitHub dark canvas)
	surfaceUp = "#161B22" // one step off surface: window chrome, tracks
	gridline  = "#21262D" // hairline grid, recessive
	baseline  = "#30363D" // axis rule
	inkHi     = "#E6EDF3" // primary ink
	inkMid    = "#8B949E" // secondary ink
	inkLow    = "#6E7681" // muted: axis ticks, captions
	deEmph    = "#484F58" // de-emphasis gray: "Other", spent sparkline
	accent    = "#3FB950" // phosphor green: the data
	accentDim = "#1A7F37" // accent, receded
	// trackGreen is the unfilled meter track: a dimmer step of the accent's own
	// ramp, so meter state reads across the whole bar. 2.56:1 on surface, above
	// the 2:1 ordinal floor.
	trackGreen = "#116329"
	warnAmber  = "#D29922" // reserved status: warning only
)

// greenRamp is the validated ordinal ramp, light -> dark. Only for genuinely
// ordered categories; never for nominal ones.
var greenRamp = []string{"#AFF5B4", "#7EE787", "#56D364", "#3FB950", "#2EA043", "#1A7F37"}

// Fonts cannot be fetched from inside an SVG that GitHub proxies through camo,
// so the stack has to resolve locally on the reader's machine.
const monoStack = "ui-monospace,SFMono-Regular,'SF Mono',Menlo,Consolas,'DejaVu Sans Mono',monospace"

// charAdvance is the width of one glyph as a fraction of font size. Every mono
// face in the stack sits within a hair of 0.6; layout that must be exact uses
// text-anchor instead of this estimate.
const charAdvance = 0.6

// Mark specs, from the dataviz mark table.
const (
	barRadius   = 4.0 // rounded data-end, square at the baseline
	surfaceGap  = 2.0 // gap in the surface color between touching marks
	hairline    = 1.0 // gridlines and axes, solid
	barThickMax = 24.0
)

// textWidth estimates the rendered width of s at the given font size.
func textWidth(s string, fontSize float64) float64 {
	return float64(len([]rune(s))) * fontSize * charAdvance
}

// fitsInside reports whether s can sit inside a mark of the given width with
// padding on both sides. A label that does not fit moves outside the mark
// rather than being clipped.
func fitsInside(s string, fontSize, markWidth, padding float64) bool {
	return textWidth(s, fontSize)+2*padding <= markWidth
}
