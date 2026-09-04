package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The archive is the one piece of state that outlives a run: the runner is thrown
// away, so a delta the archive loses is a delta nobody can recompute. These tests
// cover the round trip, the same-day replace, and the tolerance window, because a
// silent fault in any of the three produces cards that still look plausible.

func snap(date string, commits int) Snapshot {
	return Snapshot{Date: date, Commits: commits, Contributions: commits * 2}
}

func TestHistoryRoundTripsThroughDiskInDateOrder(t *testing.T) {
	path := t.TempDir() + "/history.jsonl"
	// Written out of order on purpose: the loader sorts, so a hand-edited archive
	// cannot put a newer reading in front of an older one and skew every baseline.
	if err := writeHistory(path, []Snapshot{snap("2026-03-01", 10), snap("2026-01-01", 5), snap("2026-02-01", 7)}); err != nil {
		t.Fatalf("writeHistory: %v", err)
	}

	got, err := loadHistory(path)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("loaded %d readings, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Date >= got[i].Date {
			t.Errorf("archive is not oldest-first at %d: %s then %s", i, got[i-1].Date, got[i].Date)
		}
	}
	if got[0].Commits != 5 || got[2].Commits != 10 {
		t.Errorf("values did not survive the trip: %+v", got)
	}
	// One line per reading is the whole reason for JSONL: a day's addition has to
	// be a one-line diff.
	if n := strings.Count(strings.TrimSpace(string(mustRead(t, path))), "\n") + 1; n != 3 {
		t.Errorf("archive is %d lines for 3 readings", n)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLoadHistoryTreatsAMissingFileAsAFirstRun(t *testing.T) {
	got, err := loadHistory(t.TempDir() + "/not-written-yet.jsonl")
	if err != nil {
		t.Errorf("a missing archive is the first run, not an error: %v", err)
	}
	if got != nil {
		t.Errorf("want no readings, got %v", got)
	}
}

func TestLoadHistoryRefusesACorruptLine(t *testing.T) {
	// An archive that quietly drops a line rots one reading at a time while every
	// delta it produces goes on looking reasonable, so both of these must be loud.
	cases := map[string]string{
		"not json": `{"date":"2026-01-01","commits":5}` + "\n" + `{oops` + "\n",
		"bad date": `{"date":"01/01/2026","commits":5}` + "\n",
		"no date":  `{"commits":5}` + "\n",
	}
	dir := t.TempDir()
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := dir + "/" + strings.ReplaceAll(name, " ", "-") + ".jsonl"
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := loadHistory(path); err == nil {
				t.Error("want an error, got none")
			}
		})
	}
}

func TestMergeSnapshotReplacesTheSameDate(t *testing.T) {
	hist := []Snapshot{snap("2026-01-01", 5), snap("2026-01-02", 7)}

	// A dispatch after the scheduled run is the same day seen later, which is the
	// more complete truth — it replaces rather than appends.
	got := mergeSnapshot(hist, snap("2026-01-02", 9))
	if len(got) != 2 {
		t.Fatalf("same-date merge grew the archive to %d readings", len(got))
	}
	if got[1].Commits != 9 {
		t.Errorf("the later reading did not win: %+v", got[1])
	}

	got = mergeSnapshot(got, snap("2025-12-31", 1))
	if len(got) != 3 {
		t.Fatalf("a new date should append, got %d readings", len(got))
	}
	if got[0].Date != "2025-12-31" {
		t.Errorf("append did not re-sort: %s is first", got[0].Date)
	}
}

func TestBaselineTakesTheNearestReadingInsideTheTolerance(t *testing.T) {
	s := &Stats{GeneratedAt: renderedAt, History: []Snapshot{
		snap("2026-06-01", 100), // ~94d old
		snap("2026-08-05", 200), // 29d old
		snap("2026-08-20", 300), // 14d old
		snap("2026-09-03", 400), // today, never a baseline for itself
	}}

	base, age, ok := s.Baseline(30)
	if !ok {
		t.Fatal("a 29-day-old reading is inside the 30-day window")
	}
	if base.Commits != 200 {
		t.Errorf("picked the %d-commit reading, want the nearest to 30 days", base.Commits)
	}
	// The age reported has to be the real one, or the card claims a 30-day move it
	// measured over some other span.
	if age != 29 {
		t.Errorf("reported age %dd, want the reading's true age of 29d", age)
	}

	// Nothing near a year: 94 days out is well past 30/2+1, so no claim is made.
	far := &Stats{GeneratedAt: renderedAt, History: []Snapshot{snap("2026-06-01", 100)}}
	if _, _, ok := far.Baseline(30); ok {
		t.Error("a 94-day-old reading should not back a 30-day delta")
	}
	if _, _, ok := (&Stats{GeneratedAt: renderedAt}).Baseline(30); ok {
		t.Error("an empty archive cannot produce a baseline")
	}
}

func TestTrendReadsWithoutColour(t *testing.T) {
	// Direction is a glyph and a sign so it survives greyscale, and a fall is
	// ordinary on a rolling window rather than an alarm.
	for _, tc := range []struct {
		now, then, age int
		want           string
	}{
		{1200, 1130, 27, "▲ 70 in 27d"},
		{1130, 1200, 27, "▼ 70 in 27d"},
		{1200, 1200, 27, "flat over 27d"},
		{2500, 1000, 30, "▲ 1,500 in 30d"},
	} {
		if got := trend(tc.now, tc.then, tc.age); got != tc.want {
			t.Errorf("trend(%d,%d,%d) = %q, want %q", tc.now, tc.then, tc.age, got, tc.want)
		}
	}
}

func TestLayoutCityRowsBalancesTheStreets(t *testing.T) {
	// Balanced, not greedy: the last street must never be a stub of three
	// buildings beside a lot of empty asphalt.
	for _, tc := range []struct{ n, budget, rows, cols int }{
		{0, 8, 1, 0},
		{8, 8, 1, 8},
		{9, 8, 2, 5},
		{19, 8, 3, 7},
		{30, 8, 4, 8},
	} {
		rows, cols := layoutCityRows(tc.n, tc.budget)
		if rows != tc.rows || cols != tc.cols {
			t.Errorf("layoutCityRows(%d,%d) = %dx%d, want %dx%d", tc.n, tc.budget, rows, cols, tc.rows, tc.cols)
		}
		if rows*cols < tc.n {
			t.Errorf("layoutCityRows(%d,%d) = %dx%d, which cannot hold %d", tc.n, tc.budget, rows, cols, tc.n)
		}
		if cols > tc.budget {
			t.Errorf("layoutCityRows(%d,%d) returned %d columns, over the budget", tc.n, tc.budget, cols)
		}
	}
}

func TestBandsOfCutsAlwaysStrictlyIncrease(t *testing.T) {
	// Equal cuts would print an empty band in the legend and give two swatches the
	// same meaning, so a flat distribution has to be spread apart.
	for name, in := range map[string][]int{
		"empty":      nil,
		"quiet year": {1, 1, 1, 1, 1, 1},
		"one value":  {7},
		"two tones":  {1, 1, 1, 9},
		"spread":     {1, 2, 3, 4, 5, 6, 7, 8, 20},
	} {
		t.Run(name, func(t *testing.T) {
			b := bandsOf(in)
			if b[0] < 1 {
				t.Errorf("first cut is %d; band 1 must start at 1", b[0])
			}
			if b[0] >= b[1] || b[1] >= b[2] {
				t.Errorf("cuts %v do not strictly increase", b)
			}
			// heatLevel must agree with the cuts it was handed: 0 only for a quiet
			// day, and never above the four bands the legend draws.
			if heatLevel(0, b) != 0 {
				t.Error("a day with nothing on it is not band 1")
			}
			for _, v := range []int{1, b[0], b[0] + 1, b[1], b[2], b[2] + 1, 10000} {
				if l := heatLevel(v, b); l < 1 || l > 4 {
					t.Errorf("heatLevel(%d, %v) = %d, outside the four bands", v, b, l)
				}
			}
			if heatLevel(b[0], b) != 1 || heatLevel(b[0]+1, b) != 2 || heatLevel(b[2]+1, b) != 4 {
				t.Errorf("cuts %v are not inclusive upper bounds", b)
			}
		})
	}
}

func TestDayCountGetsTheGrammarRight(t *testing.T) {
	// "1 days" was on the live profile in four places.
	for in, want := range map[int]string{0: "0 days", 1: "1 day", 2: "2 days", 1000: "1,000 days"} {
		if got := dayCount(in); got != want {
			t.Errorf("dayCount(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestCommitStampsAreOnlyTheOwnersAndAreSorted(t *testing.T) {
	raw := mustRead(t, "../../testdata/profile.json")
	resp, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := resp.toStats(renderedAt, nil)

	// Count what the fixture actually offers, so the filter is measured against the
	// data rather than against a number pasted into the test.
	want, foreign := 0, 0
	for _, n := range resp.Data.User.Repositories.Nodes {
		if n.DefaultBranchRef == nil || n.DefaultBranchRef.Target == nil {
			continue
		}
		for _, cm := range n.DefaultBranchRef.Target.Recent.Nodes {
			if cm.Author == nil || cm.Author.User == nil {
				continue
			}
			if strings.EqualFold(cm.Author.User.Login, resp.Data.User.Login) {
				want++
			} else {
				foreign++
			}
		}
	}
	if foreign == 0 {
		t.Fatal("the fixture has no commit by anyone else, so the attribution filter is untested")
	}
	if len(s.CommitStamps) != want {
		t.Errorf("kept %d stamps, want the %d attributed to %s", len(s.CommitStamps), want, resp.Data.User.Login)
	}
	// Deterministic order, or the same data renders two different cards.
	for i := 1; i < len(s.CommitStamps); i++ {
		if s.CommitStamps[i-1].At.After(s.CommitStamps[i].At) {
			t.Fatalf("stamps are not sorted at %d", i)
		}
	}
}

func TestRhythmSummaryDisclosesWhichClockItRead(t *testing.T) {
	utc := func(n int) []CommitStamp {
		out := make([]CommitStamp, n)
		for i := range out {
			out[i] = CommitStamp{At: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
		}
		return out
	}
	withOffset := CommitStamp{At: time.Date(2026, 9, 1, 12, 0, 0, 0, time.FixedZone("+02", 2*3600)), Offset: 2 * 3600}

	defer func(prev int) { tzFallback = prev }(tzFallback)

	tzFallback = 0
	if got := (&Stats{CommitStamps: utc(3)}).mustSummary(t).Clock(); !strings.HasPrefix(got, "UTC —") {
		t.Errorf("an all-UTC sample should say so plainly, got %q", got)
	}
	tzFallback = 2
	if got := (&Stats{CommitStamps: utc(3)}).mustSummary(t).Clock(); !strings.Contains(got, "UTC+2") {
		t.Errorf("the applied fallback must be named, got %q", got)
	}
	tzFallback = 0
	mixed := append(utc(2), withOffset)
	if got := (&Stats{CommitStamps: mixed}).mustSummary(t).Clock(); !strings.Contains(got, "except 2 stamps") {
		t.Errorf("a partial sample must count the UTC stamps, got %q", got)
	}
	if got := (&Stats{CommitStamps: []CommitStamp{withOffset}}).mustSummary(t).Clock(); !strings.Contains(got, "own clock") {
		t.Errorf("a fully-offset sample reads in its own clock, got %q", got)
	}
}

// mustSummary keeps the clock assertions above to one line each.
func (s *Stats) mustSummary(t *testing.T) rhythmSummary {
	t.Helper()
	_, sum := s.RhythmGrid()
	if sum.Sample != len(s.CommitStamps) {
		t.Fatalf("summary counted %d of %d stamps", sum.Sample, len(s.CommitStamps))
	}
	return sum
}

func TestRhythmGridBinsMondayFirst(t *testing.T) {
	// 2026-09-06 is a Sunday, which must land on the last row rather than the
	// first, or the weekend is split across the top and bottom edges.
	sunday := CommitStamp{At: time.Date(2026, 9, 6, 23, 0, 0, 0, time.UTC)}
	monday := CommitStamp{At: time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)}
	grid, sum := (&Stats{CommitStamps: []CommitStamp{sunday, monday, monday}}).RhythmGrid()

	if grid[6][23] != 1 {
		t.Errorf("Sunday 23:00 did not land on the last row: %v", grid[6])
	}
	if grid[0][9] != 2 {
		t.Errorf("Monday 09:00 did not land on the first row: %v", grid[0])
	}
	if sum.PeakDay() != "Mon" || sum.PeakHour != 9 || sum.Peak != 2 {
		t.Errorf("peak is %s %02d:00 x%d, want Mon 09:00 x2", sum.PeakDay(), sum.PeakHour, sum.Peak)
	}
}

func TestCalendarPeakAgreesWithTheDays(t *testing.T) {
	s := &Stats{Days: []Day{
		{Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Count: 3},
		{Date: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), Count: 0},
		{Date: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), Count: 11},
		{Date: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), Count: 0},
	}}
	busiest, quiet := s.CalendarPeak()
	if busiest.Count != 11 || busiest.Date.Day() != 3 {
		t.Errorf("busiest is %d on the %d", busiest.Count, busiest.Date.Day())
	}
	if quiet != 2 {
		t.Errorf("counted %d quiet days, want 2", quiet)
	}
	if _, q := (&Stats{}).CalendarPeak(); q != 0 {
		t.Error("an empty calendar has no quiet days to count")
	}
}

func TestMultiRowCityNamesEveryBuildingAndStaysReadable(t *testing.T) {
	// Nineteen repos in one street would be a 2,000px card that a README renders
	// at about 860, shrinking a 9.5px label to four. The whole point of wrapping is
	// that nothing has to be dropped and nothing has to shrink.
	s := &Stats{Login: "nobody", GeneratedAt: renderedAt}
	for i := 0; i < 19; i++ {
		s.Repos = append(s.Repos, Repo{
			Name:     "repo-" + string(rune('a'+i)),
			Commits:  300 - i*12,
			SizeKB:   500,
			PushedAt: renderedAt.AddDate(0, 0, -i*10),
		})
	}
	s.RepoCount = len(s.Repos)

	blocks := s.CityBlocks()
	if len(blocks) != 19 {
		t.Fatalf("skyline drew %d of 19 buildings", len(blocks))
	}
	svg := renderCity(s)
	for _, r := range blocks {
		if !strings.Contains(svg, esc(r.Name)) {
			t.Errorf("multi-row city never names %s", r.Name)
		}
	}

	w, h := svgSize(t, svg)
	if w > 900 {
		t.Errorf("card is %gpx wide; a README column is about 860", w)
	}
	// A fractional box makes the renderer draw every hairline twice at half
	// opacity, which reads as a blurry card.
	if w != float64(int(w)) || h != float64(int(h)) {
		t.Errorf("card geometry is fractional: %gx%g", w, h)
	}
}

// svgSize reads the declared width and height off the root element.
func svgSize(t *testing.T, svg string) (w, h float64) {
	t.Helper()
	num := func(attr string) float64 {
		_, rest, ok := strings.Cut(svg, attr+`="`)
		if !ok {
			t.Fatalf("no %s attribute", attr)
		}
		val, _, _ := strings.Cut(rest, `"`)
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			t.Fatalf("%s=%q: %v", attr, val, err)
		}
		return f
	}
	return num("width"), num("height")
}

func TestEveryCardHasWholePixelGeometry(t *testing.T) {
	for name, svg := range cards(loadFixture(t)) {
		t.Run(name, func(t *testing.T) {
			w, h := svgSize(t, svg)
			if w != float64(int(w)) || h != float64(int(h)) {
				t.Errorf("%s is %gx%g; a fractional box draws every hairline twice", name, w, h)
			}
			if w <= 0 || h <= 0 {
				t.Errorf("%s has no area: %gx%g", name, w, h)
			}
		})
	}
}
