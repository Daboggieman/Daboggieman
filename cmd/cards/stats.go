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
// CommitStamp is when one commit was written, in whatever clock the API reported.
//
// GitHub types Commit.committedDate as GitTimestamp, which the reference describes
// as "not converted in UTC" — so the string normally carries the offset the commit
// was actually made in and the parsed hour is already the author's own. Some
// histories come back normalised to Z regardless (CI runners commit with TZ=UTC),
// and reading those as local would quietly move a night of work into the afternoon.
// So the offset is kept rather than discarded: the card counts how many stamps
// arrived with a real offset, states it, and -tz shifts only the ones that did not.
// YearRow is one calendar year binned into months. Months rather than days
// because a decade of days is 3,650 cells, which at any size a README will render
// is texture instead of data.
type YearRow struct {
	Year   int
	Months [12]int
	Total  int
	Active int // days with at least one contribution
}

// Peak is the busiest month of the year and its index.
func (y YearRow) Peak() (month int, count int) {
	for i, v := range y.Months {
		if v > count {
			month, count = i, v
		}
	}
	return month, count
}

type CommitStamp struct {
	At     time.Time
	Offset int // seconds east of UTC, exactly as the timestamp declared it
}

// tzFallback is the offset in hours applied to stamps that arrived normalised to
// UTC. Zero by default: guessing a clock is worse than reporting which clock was
// read, so this only moves when the profile's owner sets -tz.
var tzFallback int

// Local puts a stamp on the clock the rhythm card should read it in.
func (c CommitStamp) Local() time.Time {
	if c.Offset == 0 && tzFallback != 0 {
		return c.At.Add(time.Duration(tzFallback) * time.Hour)
	}
	return c.At
}

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

	// History is the archive on disk, oldest first, loaded before rendering. It is
	// the only source of a baseline: see history.go.
	History []Snapshot

	// Years is the multi-year record, oldest first, one row per calendar year.
	Years []YearRow

	// CreatedAt is when the account was opened, so the year grid can say where the
	// record actually starts rather than implying the empty years are quiet ones.
	CreatedAt time.Time

	// CommitStamps is a sample of commit timestamps off the default branches,
	// attributed to this login. It is a sample, not the whole history, and the
	// rhythm card says so.
	CommitStamps []CommitStamp
}

// maxLanguageSlots caps the language chart. Past the cap the tail folds into
// "Other" rather than growing the category count.
const maxLanguageSlots = 6

// maxCityBlocks caps how many buildings the skyline draws; maxCityCols caps how
// many stand on one street before it wraps.
//
// The column budget is the width argument: 40px of padding plus 105px per
// building puts eight at 880px, which matches the terminal card and still lands
// under the ~860px a README column renders an image at, so 9.5px labels arrive
// close to full size. Past eight the street wraps instead of stretching, so width
// stops growing and the limit becomes vertical: 30 is roughly four streets, past
// which the card is taller than a viewport and the table view is the better read.
var (
	maxCityBlocks = 30
	maxCityCols   = 8
)

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
