package main

import (
	"sort"
	"time"
)

// Day is one cell of the contribution calendar.
type Day struct {
	Date  time.Time
	Count int
}

// Language is one aggregated language share across owned, non-fork repos.
type Language struct {
	Name  string
	Bytes int
	Share float64 // 0..1
}

// Repo is the slice of repository data the cards use.
type Repo struct {
	Name     string
	Stars    int
	PushedAt time.Time
	Primary  string
}

// Stats is everything the cards render. It is the only thing the render layer
// sees, so cards stay testable without a network.
type Stats struct {
	Login    string
	Name     string
	Location string

	Followers   int
	PublicRepos int
	Stars       int

	Commits            int
	PullRequests       int
	Issues             int
	Reviews            int
	TotalContributions int

	CurrentStreak int
	LongestStreak int

	Days      []Day
	Languages []Language
	Repos     []Repo

	GeneratedAt time.Time
}

// maxLanguageSlots caps the language chart. Past the cap the tail folds into
// "Other" rather than growing the category count.
const maxLanguageSlots = 6

// computeStreaks returns the current and longest run of consecutive days with at
// least one contribution. days must be in ascending date order.
//
// The contribution calendar is week-aligned, so it runs to the end of the current
// week and the not-yet-happened days come back as zeros. Those are dropped before
// the walk, or every streak would read 0 from midweek onward. A zero on today
// itself does not break the current streak either: the day is still in progress
// at render time.
func computeStreaks(days []Day, today time.Time) (current, longest int) {
	days = upTo(days, today)

	run := 0
	for _, d := range days {
		if d.Count > 0 {
			run++
			if run > longest {
				longest = run
			}
			continue
		}
		run = 0
	}

	i := len(days) - 1
	if i >= 0 && days[i].Count == 0 {
		i--
	}
	for ; i >= 0 && days[i].Count > 0; i-- {
		current++
	}
	return current, longest
}

// upTo drops calendar cells dated after today.
func upTo(days []Day, today time.Time) []Day {
	cutoff := today.UTC().Truncate(24 * time.Hour)
	for i := len(days) - 1; i >= 0; i-- {
		if !days[i].Date.After(cutoff) {
			return days[:i+1]
		}
	}
	return nil
}

// aggregateLanguages sums byte counts by language, sorts by size descending,
// and folds everything past maxLanguageSlots into a single "Other" entry.
// Shares are computed against the full total, so they always sum to 1.
func aggregateLanguages(byLang map[string]int) []Language {
	total := 0
	for _, b := range byLang {
		total += b
	}
	if total == 0 {
		return nil
	}

	out := make([]Language, 0, len(byLang))
	for name, b := range byLang {
		out = append(out, Language{Name: name, Bytes: b})
	}
	// Name is the tiebreaker so equal byte counts render in a stable order.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].Name < out[j].Name
	})

	if len(out) > maxLanguageSlots {
		other := 0
		for _, l := range out[maxLanguageSlots:] {
			other += l.Bytes
		}
		out = append(out[:maxLanguageSlots:maxLanguageSlots], Language{Name: "Other", Bytes: other})
	}
	for i := range out {
		out[i].Share = float64(out[i].Bytes) / float64(total)
	}
	return out
}

// lastDays returns the trailing window of the calendar, at most count entries.
func lastDays(days []Day, count int) []Day {
	if len(days) <= count {
		return days
	}
	return days[len(days)-count:]
}

// maxCount is the largest count in a window, floored at 1 so bar scaling never
// divides by zero on a quiet stretch.
func maxCount(days []Day) int {
	m := 0
	for _, d := range days {
		if d.Count > m {
			m = d.Count
		}
	}
	if m == 0 {
		return 1
	}
	return m
}
