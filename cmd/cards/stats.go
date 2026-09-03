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
	// Commits is the default branch's history length. On a solo profile that is
	// effectively the owner's commit count, but it does count every author, so
	// the city card labels it "commits on the default branch" rather than "your
	// commits".
	Commits int
	// SizeKB is GitHub's diskUsage: the repo's size in kilobytes.
	SizeKB int
	// Private is true for a repo only the owner's token can see. Those repos are
	// counted and drawn like any other — the work is real — but nothing that
	// renders them may link to them, because the link is a 404 for every visitor.
	// What a visitor gets is the activity: commits, footprint, last push.
	Private bool
}

// PushedWithin reports whether the repo was pushed to in the trailing window.
func (r Repo) PushedWithin(now time.Time, d time.Duration) bool {
	if r.PushedAt.IsZero() {
		return false
	}
	return now.Sub(r.PushedAt) <= d
}

// Stats is everything the cards render. It is the only thing the render layer
// sees, so cards stay testable without a network.
type Stats struct {
	Login    string
	Name     string
	Location string

	Followers int
	// RepoCount is every owned non-fork repo the token could see. With a
	// repo-scoped PAT that includes private ones, so it is not "public repos".
	RepoCount int
	Stars     int

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

// maxCityBlocks caps how many buildings the skyline draws. It is a var rather than
// a const only so -buildings can raise it; nothing mutates it after start-up.
//
// The default is 9, which is where the arithmetic runs out rather than a taste
// call. The card grows sideways to fit what it is given — 40px of padding plus
// 105px per building — and a README column renders an image at roughly 860px, so
// anything wider is downscaled and the 9.5px labels shrink with it. Holding labels
// at 8px effective allows about 1020px of card, which is 9 buildings. Past that the
// skyline would be a row of unreadable rectangles, so the cap cuts instead, the
// chrome says how many of how many are standing, and the table view keeps the rest.
var maxCityBlocks = 9

// cityFreshWindow is how recently a repo must have been pushed to for its
// building to read as occupied. Roughly a month: long enough that a normal gap
// between sessions does not go dark, short enough to mean something.
const cityFreshWindow = 30 * 24 * time.Hour

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

// PrivateCount is how many of the rendered repos are private. The cards use it to
// say so out loud: a visitor reading a building they cannot open deserves to know
// why, rather than finding out by clicking a dead link.
func (s *Stats) PrivateCount() int {
	n := 0
	for _, r := range s.Repos {
		if r.Private {
			n++
		}
	}
	return n
}
