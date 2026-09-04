package main

import (
	"fmt"
	"strconv"
	"strings"
)

// compact renders a stat-tile value: comma'd below 10k, then K, then M.
func compact(v int) string {
	switch {
	case v >= 1_000_000:
		return trimZero(fmt.Sprintf("%.1f", float64(v)/1_000_000)) + "M"
	case v >= 10_000:
		return trimZero(fmt.Sprintf("%.1f", float64(v)/1_000)) + "K"
	default:
		return commas(v)
	}
}

func trimZero(s string) string { return strings.TrimSuffix(s, ".0") }

func commas(v int) string {
	s := strconv.Itoa(v)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	out = append([]string{s}, out...)
	joined := strings.Join(out, ",")
	if neg {
		return "-" + joined
	}
	return joined
}

// dayCount renders a span in days. plural() would do, except streak counts are
// worth comma-grouping once they pass a thousand, and "1 days" was on the live
// profile in four places before this existed.
func dayCount(n int) string {
	if n == 1 {
		return "1 day"
	}
	return commas(n) + " days"
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// truncateToWidth shortens s until it fits maxWidth at fontSize, marking the cut
// with an ellipsis. A label that will not fit is never allowed to run into the
// mark beside it; the full value stays in the tooltip and the README table.
func truncateToWidth(s string, fontSize, maxWidth float64) string {
	if textWidth(s, fontSize) <= maxWidth {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if textWidth(string(r)+"…", fontSize) <= maxWidth {
			return string(r) + "…"
		}
	}
	return "…"
}

func sumCounts(days []Day) int {
	total := 0
	for _, d := range days {
		total += d.Count
	}
	return total
}

// priorWindow returns the window of size count immediately before the trailing
// window of the same size, for period-over-period comparison.
func priorWindow(days []Day, count int) []Day {
	if len(days) < 2*count {
		return nil
	}
	end := len(days) - count
	return days[end-count : end]
}

// formatDelta describes a period-over-period change. Direction rides a glyph and
// a sign, never color alone, so it stays readable under any vision and never
// borrows a reserved status hue.
func formatDelta(current, prior, window int) string {
	if prior == 0 {
		if current == 0 {
			return ""
		}
		return fmt.Sprintf("new activity vs prior %dd", window)
	}
	pct := (float64(current-prior) / float64(prior)) * 100
	switch {
	case pct >= 0.5:
		return fmt.Sprintf("▲ %.0f%% vs prior %dd", pct, window)
	case pct <= -0.5:
		return fmt.Sprintf("▼ %.0f%% vs prior %dd", -pct, window)
	default:
		return fmt.Sprintf("flat vs prior %dd", window)
	}
}
