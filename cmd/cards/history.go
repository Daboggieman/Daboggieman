package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// The archive.
//
// Every number on these cards is a "right now" reading. The GraphQL API answers
// what the profile looks like today and keeps no memory of last month, so a
// delta — the thing that turns a stat into a direction — cannot be fetched, only
// remembered. Each run folds today's totals into history.jsonl, and the cards read
// their baselines back out of it.
//
// JSONL rather than one JSON document because the file is append-mostly and lives
// in git: a day's addition is a one-line diff instead of a reindented blob, and a
// line that gets corrupted or hand-edited costs that day rather than the archive.
// Re-running on the same date replaces that date's line instead of adding a
// second, so a manual workflow_dispatch after the scheduled run cannot make one
// day look like two.
//
// The keys are short and, once written, permanent: they are on disk in a committed
// file, so renaming one silently orphans every reading already taken.

// Snapshot is one day's totals.
type Snapshot struct {
	Date          string `json:"date"` // YYYY-MM-DD in UTC
	Contributions int    `json:"contributions"`
	Commits       int    `json:"commits"`
	PullRequests  int    `json:"prs"`
	Issues        int    `json:"issues"`
	Reviews       int    `json:"reviews"`
	Stars         int    `json:"stars"`
	Repos         int    `json:"repos"`
	Followers     int    `json:"followers"`
	CurrentStreak int    `json:"streak"`
	LongestStreak int    `json:"best_streak"`

	// Langs is each language's share in percentage points on the day of the
	// reading. Stored because a share is the one number on these cards that says
	// what the work is turning into rather than how much of it there was, and it
	// cannot be reconstructed later: the API answers only for today. Omitted when
	// empty so a reading taken before this field existed still round-trips.
	Langs map[string]float64 `json:"langs,omitempty"`
}

const snapshotDate = "2006-01-02"

// Snapshot takes today's reading off the render model.
func (s *Stats) Snapshot() Snapshot {
	return Snapshot{
		Date:          s.GeneratedAt.UTC().Format(snapshotDate),
		Contributions: s.TotalContributions,
		Commits:       s.Commits,
		PullRequests:  s.PullRequests,
		Issues:        s.Issues,
		Reviews:       s.Reviews,
		Stars:         s.Stars,
		Repos:         s.RepoCount,
		Followers:     s.Followers,
		CurrentStreak: s.CurrentStreak,
		LongestStreak: s.LongestStreak,
		Langs:         s.langShareMap(),
	}
}

// langShares rounds each share to two decimal places of a percent. Rounded
// because the archive is a committed text file: unrounded float64 noise in the
// fifteenth digit would make a diff on days when nothing actually moved.
// langShareMap is the share map the archive stores and the drift card reads. It
// prefers the unfolded map and falls back to the drawn slice, so a Stats built by
// hand still archives the shares it does have.
func (s *Stats) langShareMap() map[string]float64 {
	if len(s.LangShares) > 0 {
		return s.LangShares
	}
	return langShares(s.Languages)
}

// sharesOfBytes turns raw byte counts into percentage points of the whole.
func sharesOfBytes(byLang map[string]int) map[string]float64 {
	total := 0
	for _, b := range byLang {
		total += b
	}
	if total == 0 {
		return nil
	}
	out := make(map[string]float64, len(byLang))
	for name, b := range byLang {
		out[name] = math.Round(float64(b)/float64(total)*10000) / 100
	}
	return out
}

func langShares(langs []Language) map[string]float64 {
	if len(langs) == 0 {
		return nil
	}
	out := make(map[string]float64, len(langs))
	for _, l := range langs {
		out[l.Name] = math.Round(l.Share*10000) / 100
	}
	return out
}

// LangMove is one language's share at two points in time, in percentage points.
type LangMove struct {
	Name      string
	Then, Now float64
}

func (m LangMove) Delta() float64 { return m.Now - m.Then }

// LanguageDrift pairs today's shares against the nearest archived reading that
// recorded them, biggest mover first.
//
// Percentage points, not percent-of-percent: a language going 40% -> 44% moved
// four points, and calling that "10% growth" would be a different and much less
// useful claim.
//
// Both sides come from the unfolded share map, never from the drawn slice. The
// languages card folds its tail into "Other", and comparing two folded readings
// makes a language that merely crossed the fold look like it was deleted — a wrong
// number rather than a missing one. "Other" itself is excluded for the same reason:
// it is a bucket whose membership changes between readings, so its movement measures
// the fold, not the work.
//
// Ranked by the size of the move rather than by share, because that is the quantity
// this card exists to show; a caller that wants share order has the languages card
// and the table view. Languages present in only one reading still appear, since an
// entry appearing or vanishing is the most interesting kind of drift.
func (s *Stats) LanguageDrift(want int) ([]LangMove, int, bool) {
	base, age, ok := s.Baseline(want)
	if !ok {
		return nil, 0, false
	}
	then, now := withoutOther(base.Langs), withoutOther(s.langShareMap())
	if len(then) == 0 || len(now) == 0 {
		return nil, 0, false
	}

	out := make([]LangMove, 0, len(now)+len(then))
	for name, v := range now {
		out = append(out, LangMove{Name: name, Then: then[name], Now: v})
	}
	for name, v := range then {
		if _, held := now[name]; !held {
			out = append(out, LangMove{Name: name, Then: v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := math.Abs(out[i].Delta()), math.Abs(out[j].Delta())
		switch {
		case a != b:
			return a > b
		case out[i].Now != out[j].Now:
			return out[i].Now > out[j].Now
		}
		return out[i].Name < out[j].Name
	})
	return out, age, true
}

// withoutOther copies a share map with the folded bucket left out.
func withoutOther(m map[string]float64) map[string]float64 {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]float64, len(m))
	for name, v := range m {
		if name != "Other" {
			out[name] = v
		}
	}
	return out
}

// loadHistory reads the archive, oldest first.
//
// A missing file is not an error — that is the first run. A malformed line is,
// because an archive that quietly drops readings rots one line at a time while
// every delta it produces goes on looking plausible.
func loadHistory(path string) ([]Snapshot, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var out []Snapshot
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var snap Snapshot
		if err := json.Unmarshal([]byte(line), &snap); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		if _, err := time.Parse(snapshotDate, snap.Date); err != nil {
			return nil, fmt.Errorf("%s line %d: date %q is not YYYY-MM-DD", path, i+1, snap.Date)
		}
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

// mergeSnapshot folds a reading into the archive: same date replaces, new date
// appends. Replace rather than skip, because a same-day re-run is the more recent
// truth — the scheduled run at 06:17 is a partial day, and a dispatch at 23:00 is
// the one worth keeping.
func mergeSnapshot(hist []Snapshot, snap Snapshot) []Snapshot {
	for i := range hist {
		if hist[i].Date == snap.Date {
			hist[i] = snap
			return hist
		}
	}
	out := append(hist, snap)
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// writeHistory rewrites the archive. Whole-file rather than an O_APPEND write
// because a same-day re-run edits a line in place, and via a temp file plus rename
// so a process killed mid-write leaves the previous archive intact rather than
// half of a new one.
func writeHistory(path string, hist []Snapshot) error {
	var b strings.Builder
	for _, snap := range hist {
		line, err := json.Marshal(snap)
		if err != nil {
			return fmt.Errorf("encode snapshot %s: %w", snap.Date, err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// Baseline returns the archived reading nearest to want days before the render
// date, along with how old it actually is.
//
// Nearest rather than oldest-at-least-that-old, because the archive has a gap
// wherever the workflow did not run and a 26-day-old reading is a truer "about a
// month ago" than a 200-day-old one. The tolerance is half the window: past that
// the label would be describing a different span than the one it names. The real
// age comes back with the reading so the card can say 26 days when that is what it
// measured — a delta that rounds its own baseline to a rounder number is a small
// lie that compounds every time it is quoted.
func (s *Stats) Baseline(want int) (Snapshot, int, bool) {
	if len(s.History) == 0 || want <= 0 {
		return Snapshot{}, 0, false
	}
	today := s.GeneratedAt.UTC().Truncate(24 * time.Hour)

	best, bestAge, bestMiss := Snapshot{}, 0, want/2+1
	for _, snap := range s.History {
		d, err := time.Parse(snapshotDate, snap.Date)
		if err != nil {
			continue
		}
		age := int(today.Sub(d).Hours() / 24)
		if age <= 0 {
			continue // today's own reading is not a baseline for itself
		}
		miss := age - want
		if miss < 0 {
			miss = -miss
		}
		if miss < bestMiss {
			best, bestAge, bestMiss = snap, age, miss
		}
	}
	if bestAge == 0 {
		return Snapshot{}, 0, false
	}
	return best, bestAge, true
}

// trend renders a KPI's movement against a baseline, or "" when the archive cannot
// support the claim yet. Direction is a glyph and a sign, never a colour, so it
// survives greyscale and any colour vision.
//
// These are rolling-365-day totals, so a fall is ordinary rather than alarming: it
// means the window's far edge dropped a busier week than its near edge added.
func trend(now, then, age int) string {
	switch d := now - then; {
	case d > 0:
		return fmt.Sprintf("▲ %s in %dd", commas(d), age)
	case d < 0:
		return fmt.Sprintf("▼ %s in %dd", commas(-d), age)
	default:
		return fmt.Sprintf("flat over %dd", age)
	}
}

// Previous is the newest archived reading from before the render date.
func (s *Stats) Previous() (Snapshot, bool) {
	today := s.GeneratedAt.UTC().Format(snapshotDate)
	for i := len(s.History) - 1; i >= 0; i-- {
		if s.History[i].Date < today {
			return s.History[i], true
		}
	}
	return Snapshot{}, false
}

// commitSubject says what actually moved since the last archived reading.
//
// The workflow used to commit "refresh from live profile data" every single day,
// which makes a year of history unreadable: the log records that something
// happened 365 times and never what. Naming the movement turns it into a record
// worth keeping, and on the day a number does something surprising the log is
// where that shows up first.
func commitSubject(s *Stats) string {
	prev, ok := s.Previous()
	if !ok {
		return fmt.Sprintf("cards: first render, %s contributions", commas(s.TotalContributions))
	}

	var parts []string
	for _, m := range []struct {
		now, then int
		noun      string
	}{
		{s.TotalContributions, prev.Contributions, "contributions"},
		{s.Commits, prev.Commits, "commits"},
		{s.PullRequests, prev.PullRequests, "PRs"},
		{s.Reviews, prev.Reviews, "reviews"},
		{s.Stars, prev.Stars, "stars"},
		{s.RepoCount, prev.Repos, "repos"},
		{s.Followers, prev.Followers, "followers"},
	} {
		if d := m.now - m.then; d != 0 {
			parts = append(parts, fmt.Sprintf("%s %s", signed(d), m.noun))
		}
	}
	if s.CurrentStreak != prev.CurrentStreak {
		parts = append(parts, fmt.Sprintf("streak %d", s.CurrentStreak))
	}
	if s.LongestStreak != prev.LongestStreak {
		parts = append(parts, fmt.Sprintf("best streak %d", s.LongestStreak))
	}

	if len(parts) == 0 {
		return "cards: refresh, totals unchanged"
	}
	// A subject line that wraps in a log is a subject line nobody reads, so the tail
	// folds into a count rather than running on.
	subject := "cards: " + strings.Join(parts, ", ")
	for len(parts) > 1 && len(subject) > 68 {
		parts = parts[:len(parts)-1]
		subject = "cards: " + strings.Join(parts, ", ") + " and more"
	}
	return subject
}

func signed(n int) string {
	if n > 0 {
		return "+" + commas(n)
	}
	return "-" + commas(-n)
}

// trendFor renders one archived measure's movement over the last month, or "" when
// the archive is too young to have an opinion. A card that says nothing until it
// has real ground to stand on beats one that invents a baseline on day one.
func (s *Stats) trendFor(now int, pick func(Snapshot) int) string {
	base, age, ok := s.Baseline(30)
	if !ok {
		return ""
	}
	return trend(now, pick(base), age)
}
