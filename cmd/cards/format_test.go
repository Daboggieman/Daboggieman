package main

import (
	"strings"
	"testing"
)

func TestCompact(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1_210, "1,210"},
		{9_999, "9,999"},
		{10_000, "10K"},
		{12_500, "12.5K"},
		{999_999, "1000K"},
		{1_000_000, "1M"},
		{2_400_000, "2.4M"},
	}
	for _, tt := range tests {
		if got := compact(tt.in); got != tt.want {
			t.Errorf("compact(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCommas(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"}, {12, "12"}, {123, "123"}, {1234, "1,234"},
		{1234567, "1,234,567"}, {-4321, "-4,321"},
	}
	for _, tt := range tests {
		if got := commas(tt.in); got != tt.want {
			t.Errorf("commas(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "repo"); got != "1 repo" {
		t.Errorf("got %q", got)
	}
	if got := plural(0, "repo"); got != "0 repos" {
		t.Errorf("got %q", got)
	}
	if got := plural(4, "commit"); got != "4 commits" {
		t.Errorf("got %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 kB"},
		{1 << 20, "1.0 MB"},
		{5 * (1 << 20), "5.0 MB"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.in); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTruncateToWidth(t *testing.T) {
	const size = 11.0

	if got := truncateToWidth("Go", size, 100); got != "Go" {
		t.Errorf("a label that fits must come back untouched, got %q", got)
	}

	// "Jupyter Notebook" is the label that overflowed its column on the language
	// card, so it is the case worth pinning.
	got := truncateToWidth("Jupyter Notebook", size, 60)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a truncated label should be marked with an ellipsis, got %q", got)
	}
	if w := textWidth(got, size); w > 60 {
		t.Errorf("%q is %v wide, over the 60 budget", got, w)
	}

	if got := truncateToWidth("anything", size, 1); got != "…" {
		t.Errorf("an impossible budget should collapse to an ellipsis, got %q", got)
	}
}

func TestFormatDelta(t *testing.T) {
	tests := []struct {
		name           string
		current, prior int
		want           string
	}{
		{"no history and no activity says nothing", 0, 0, ""},
		{"no history but activity now", 40, 0, "new activity vs prior 30d"},
		{"a rise carries an up glyph and a sign", 120, 90, "▲ 33% vs prior 30d"},
		{"a fall carries a down glyph", 60, 90, "▼ 33% vs prior 30d"},
		{"a rounding-level change reads flat", 100, 100, "flat vs prior 30d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDelta(tt.current, tt.prior, 30); got != tt.want {
				t.Errorf("formatDelta(%d, %d, 30) = %q, want %q",
					tt.current, tt.prior, got, tt.want)
			}
		})
	}
}

func TestPriorWindow(t *testing.T) {
	days := make([]Day, 10)
	for i := range days {
		days[i] = Day{Count: i}
	}

	got := priorWindow(days, 3)
	if len(got) != 3 {
		t.Fatalf("got %d days, want 3", len(got))
	}
	// The trailing 3 are 7,8,9; the window before it is 4,5,6.
	if got[0].Count != 4 || got[2].Count != 6 {
		t.Errorf("prior window is %v, want counts 4..6", got)
	}

	if got := priorWindow(days, 6); got != nil {
		t.Errorf("too little history for a comparison must return nil, got %d days", len(got))
	}
}

func TestSumCounts(t *testing.T) {
	if got := sumCounts([]Day{{Count: 2}, {Count: 5}, {Count: 0}}); got != 7 {
		t.Errorf("got %d, want 7", got)
	}
	if got := sumCounts(nil); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}
