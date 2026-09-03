package main

import (
	"strings"
	"testing"
)

func TestEscapesEveryXMLMetacharacter(t *testing.T) {
	got := esc(`a & b < c > d " e ' f`)
	want := `a &amp; b &lt; c &gt; d &quot; e &apos; f`
	if got != want {
		t.Errorf("esc = %q, want %q", got, want)
	}
	// A repo name is user-controlled text that lands in both attributes and
	// element content, so it must never be able to close a tag.
	if strings.ContainsAny(esc(`<script>x</script>`), "<>") {
		t.Error("esc left angle brackets in its output")
	}
}

func TestN(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"}, {12, "12"}, {12.5, "12.5"}, {-3.25, "-3.25"},
		{0.0000001, "0.0000001"}, // never scientific notation, which SVG rejects
	}
	for _, tt := range tests {
		if got := n(tt.in); got != tt.want {
			t.Errorf("n(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBarPathsAreEmptyWhenThereIsNothingToDraw(t *testing.T) {
	if got := hBarPath(0, 0, 0, 10, 4); got != "" {
		t.Errorf("a zero-width bar should emit no path, got %q", got)
	}
	if got := hBarPath(0, 0, 10, 0, 4); got != "" {
		t.Errorf("a zero-height bar should emit no path, got %q", got)
	}
	if got := vBarPath(0, 0, 10, 0, 4); got != "" {
		t.Errorf("a zero-height column should emit no path, got %q", got)
	}
}

func TestHBarPathIsSquareAtTheBaseline(t *testing.T) {
	// A horizontal bar grows rightward from x, so the left edge is the baseline
	// and must stay square; only the data-end is rounded.
	d := hBarPath(20, 100, 200, 10, 4)
	if !strings.HasPrefix(d, "M20 100") {
		t.Errorf("path starts %q, want it anchored at the baseline corner", d[:12])
	}
	if !strings.HasSuffix(d, "H20Z") {
		t.Errorf("path ends %q, want a square return to the baseline", d[len(d)-6:])
	}
	if strings.Count(d, "A") != 2 {
		t.Errorf("want exactly two arcs (the data-end corners), got %d in %q",
			strings.Count(d, "A"), d)
	}
}

func TestVBarPathIsSquareAtTheBaseline(t *testing.T) {
	d := vBarPath(20, 50, 10, 40, 4)
	// The baseline of a column is its bottom, at y+h.
	if !strings.HasPrefix(d, "M20 90") {
		t.Errorf("path starts %q, want it anchored at y+h", d[:10])
	}
	if !strings.HasSuffix(d, "V90Z") {
		t.Errorf("path ends %q, want a square return to the baseline", d[len(d)-6:])
	}
	if strings.Count(d, "A") != 2 {
		t.Errorf("want exactly two arcs (the cap corners), got %d", strings.Count(d, "A"))
	}
}

func TestBarRadiusClampsOnTinyMarks(t *testing.T) {
	// A 3px bar cannot carry a 4px radius; clamping is what keeps the path from
	// turning inside out on a near-zero value.
	if d := hBarPath(0, 0, 3, 10, 4); !strings.Contains(d, "A3 3") {
		t.Errorf("radius should clamp to the width, got %q", d)
	}
	if d := hBarPath(0, 0, 100, 5, 4); !strings.Contains(d, "A2.5 2.5") {
		t.Errorf("radius should clamp to half the thickness, got %q", d)
	}
}
