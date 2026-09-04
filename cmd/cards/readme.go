package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"
)

// The generator owns only the text between these markers. Everything else in the
// README is hand-written and is never touched.
const (
	tableStart = "<!-- cards:table:start -->"
	tableEnd   = "<!-- cards:table:end -->"
)

// updateReadme rewrites the marked block with a plain-markdown twin of the
// charts. The cards cannot carry a hover layer — GitHub serves them through an
// image proxy — so this table is how every plotted value stays readable without
// relying on color or on an interaction that is not available.
func updateReadme(path string, s *Stats) (bool, error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	startIdx := bytes.Index(original, []byte(tableStart))
	endIdx := bytes.Index(original, []byte(tableEnd))
	switch {
	case startIdx < 0 && endIdx < 0:
		return false, nil // nothing to refresh in this README
	case startIdx < 0 || endIdx < 0:
		return false, fmt.Errorf("%s has an unpaired cards:table marker", path)
	case endIdx < startIdx:
		return false, fmt.Errorf("%s has cards:table markers in the wrong order", path)
	}

	replacement := tableStart + "\n\n" + renderTableView(s) + "\n" + tableEnd
	updated := make([]byte, 0, len(original))
	updated = append(updated, original[:startIdx]...)
	updated = append(updated, replacement...)
	updated = append(updated, original[endIdx+len(tableEnd):]...)

	if bytes.Equal(original, updated) {
		return false, nil
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// renderTableView is the accessible twin of the language, activity and city cards.
func renderTableView(s *Stats) string {
	var b strings.Builder

	// The terminal card's live line, first because it is the only reading here that
	// answers "is this profile alive right now" rather than "how big is the total".
	if focus, ok := s.Focus(); ok {
		b.WriteString("| Right now | Reading |\n|---|---|\n")
		fmt.Fprintf(&b, "| Focus | %s |\n", focus)
		fmt.Fprintf(&b, "| Last 7 days | %s |\n\n", plural(s.ThisWeek(), "contribution"))
	}

	b.WriteString("| Language | Share | Tracked size |\n|---|---:|---:|\n")
	if len(s.Languages) == 0 {
		b.WriteString("| _no data_ | — | — |\n")
	}
	for _, l := range s.Languages {
		fmt.Fprintf(&b, "| %s | %.1f%% | %s |\n", l.Name, l.Share*100, humanBytes(l.Bytes))
	}

	// The drift card in words. Percentage points in both value columns, because a
	// share and a change in a share are the same unit and belong on one scale.
	if moves, age, ok := s.LanguageDrift(30); ok {
		fmt.Fprintf(&b, "\n| Language | Share %s ago | Share now | Move |\n|---|---:|---:|---:|\n",
			dayCount(age))
		for _, m := range moves {
			fmt.Fprintf(&b, "| %s | %.1f%% | %.1f%% | %s |\n", m.Name, m.Then, m.Now, points(m.Delta()))
		}
	}

	// The third column is the same movement the cards print, sourced from the same
	// archive. It is empty until history.jsonl has a reading old enough to compare
	// against, which is the honest answer on a first run.
	b.WriteString("\n| Metric | Last 365 days | Change |\n|---|---:|---:|\n")
	rows := []struct {
		label string
		value string
		pick  func(Snapshot) int
		now   int
	}{
		{"Contributions", commas(s.TotalContributions), func(p Snapshot) int { return p.Contributions }, s.TotalContributions},
		{"Commits", commas(s.Commits), func(p Snapshot) int { return p.Commits }, s.Commits},
		{"Pull requests", commas(s.PullRequests), func(p Snapshot) int { return p.PullRequests }, s.PullRequests},
		{"Issues", commas(s.Issues), func(p Snapshot) int { return p.Issues }, s.Issues},
		{"Reviews", commas(s.Reviews), func(p Snapshot) int { return p.Reviews }, s.Reviews},
		{"Current streak", dayCount(s.CurrentStreak), func(p Snapshot) int { return p.CurrentStreak }, s.CurrentStreak},
		{"Longest streak", dayCount(s.LongestStreak), func(p Snapshot) int { return p.LongestStreak }, s.LongestStreak},
		{"Repositories", commas(s.RepoCount), func(p Snapshot) int { return p.Repos }, s.RepoCount},
		{"Stars earned", commas(s.Stars), func(p Snapshot) int { return p.Stars }, s.Stars},
		{"Followers", commas(s.Followers), func(p Snapshot) int { return p.Followers }, s.Followers},
	}
	for _, r := range rows {
		change := "—"
		if r.pick != nil {
			if t := s.trendFor(r.now, r.pick); t != "" {
				change = t
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", r.label, r.value, change)
	}
	// The collaboration card's headline is a ratio, and a ratio read off a bar chart
	// is a guess. The share belongs in the value cell, not in the change column —
	// one column, one quantity.
	if acts := s.PullRequests + s.Reviews + s.Issues; s.TotalContributions > 0 {
		change := "—"
		if t := s.trendFor(acts, func(p Snapshot) int {
			return p.PullRequests + p.Reviews + p.Issues
		}); t != "" {
			change = t
		}
		fmt.Fprintf(&b, "| Collaborative acts | %s (%.1f%% of all) | %s |\n",
			commas(acts), float64(acts)/float64(s.TotalContributions)*100, change)
	}

	// The calendar and the rhythm card are both sequential heatmaps, and a heatmap
	// is the one form that says nothing at all without color. Their headline
	// readings go here as words so the page still carries them.
	if busiest, quiet := s.CalendarPeak(); len(s.Days) > 0 {
		b.WriteString("\n| Calendar | Value |\n|---|---:|\n")
		fmt.Fprintf(&b, "| Busiest day | %s (%s) |\n",
			busiest.Date.Format("2 Jan 2006"), plural(busiest.Count, "contribution"))
		fmt.Fprintf(&b, "| Days with nothing | %s of %s |\n",
			commas(quiet), plural(len(s.Days), "day"))
		fmt.Fprintf(&b, "| Days with something | %s |\n", commas(len(s.Days)-quiet))
	}

	if _, sum := s.RhythmGrid(); sum.Sample > 0 {
		b.WriteString("\n| Commit rhythm | Value |\n|---|---:|\n")
		fmt.Fprintf(&b, "| Busiest slot | %s %02d:00–%02d:59 (%s) |\n",
			sum.PeakDay(), sum.PeakHour, sum.PeakHour, plural(sum.Peak, "commit"))
		fmt.Fprintf(&b, "| Sample | %s, %s to %s |\n", plural(sum.Sample, "commit"),
			sum.Oldest.Format("2 Jan 2006"), sum.Newest.Format("2 Jan 2006"))
		fmt.Fprintf(&b, "| Weekend work | %s of the sample |\n", sum.Share(sum.Weekend))
		fmt.Fprintf(&b, "| Nights, 22:00–06:00 | %s of the sample |\n", sum.Share(sum.Night))
		fmt.Fprintf(&b, "| Clock | %s |\n", sum.Clock())
	}

	// The year grid as digits. The card's cells are quartile bands; these are the
	// numbers those bands stand for, which is the reading a band cannot give.
	if len(s.Years) > 0 {
		b.WriteString("\n| Year | Contributions | Active days | Busiest month |\n|---|---:|---:|---|\n")
		best, _ := s.BestYear()
		for _, y := range s.Years {
			month, count := y.Peak()
			label := fmt.Sprintf("%d", y.Year)
			if y.Year == best.Year {
				label = "**" + label + "**"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s (%s) |\n", label, commas(y.Total),
				commas(y.Active), time.Month(month + 1).String()[:3], commas(count))
		}
		if !s.CreatedAt.IsZero() {
			fmt.Fprintf(&b, "\n<sub>The account opened %s, so anything before that is absent rather than quiet.</sub>\n",
				s.CreatedAt.Format("2 January 2006"))
		}
	}

	// Every repo's height, footprint and lit state, so the city is readable with
	// images off — and so a repo the skyline could not fit is still on the page.
	blocks := s.RankedRepos()
	if len(blocks) > 0 {
		b.WriteString("\n| Building | Commits | Size | Stars | Last push | State | Access |\n|---|---:|---:|---:|---|---|---|\n")
		for _, r := range blocks {
			state := "dormant"
			if r.PushedWithin(s.GeneratedAt, cityFreshWindow) {
				state = "occupied"
			}
			// A private repo is named and measured but never linked: the URL is a
			// 404 for every visitor, and a dead link reads as a broken profile
			// rather than as a closed door.
			name, access := fmt.Sprintf("[%s](https://github.com/%s/%s)", r.Name, s.Login, r.Name), "public"
			if r.Private {
				name, access = r.Name, "private"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
				name, commas(r.Commits), humanBytes(r.SizeKB*1024),
				commas(r.Stars), r.PushedAt.Format("2 Jan 2006"), state, access)
		}
		drawn := ""
		if len(blocks) > maxCityBlocks {
			drawn = fmt.Sprintf(" The skyline draws the top %d; the rest are listed here.", maxCityBlocks)
		}
		fmt.Fprintf(&b, "\n<sub>Building height is commits on the default branch, footprint is repo size, "+
			"and a lit facade means pushed within %s.%s Private repositories are listed for their "+
			"activity only — they are not linked, because the link would 404 for everyone but me.</sub>\n",
			cityAge(), drawn)
	}

	// plural() appends a bare "s", which "repository" does not take.
	scope := "Public activity only."
	if n := s.PrivateCount(); n > 0 {
		noun := "private repositories"
		if n == 1 {
			noun = "private repository"
		}
		scope = fmt.Sprintf("Includes %d %s, counted for activity but not linked.", n, noun)
	}
	fmt.Fprintf(&b, "\n<sub>Generated by <code>./cmd/cards</code> on %s. %s</sub>\n",
		s.GeneratedAt.Format("2 January 2006"), scope)
	return b.String()
}
