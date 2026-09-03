package main

import (
	"testing"
	"time"
)

func day(s string, count int) Day {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return Day{Date: d, Count: count}
}

func at(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestComputeStreaks(t *testing.T) {
	tests := []struct {
		name            string
		days            []Day
		today           string
		current, longest int
	}{
		{
			name:  "empty calendar",
			days:  nil,
			today: "2026-09-03",
		},
		{
			name: "run ends on today",
			days: []Day{
				day("2026-08-31", 0), day("2026-09-01", 2),
				day("2026-09-02", 5), day("2026-09-03", 1),
			},
			today:   "2026-09-03",
			current: 3, longest: 3,
		},
		{
			// The bug this package shipped once: the contribution calendar is
			// week-aligned, so it runs to Saturday and the days after today come
			// back as zeros. Walking backwards from the last cell stopped on the
			// first of those and reported no streak at all.
			name: "trailing future cells do not kill the streak",
			days: []Day{
				day("2026-09-01", 2), day("2026-09-02", 5), day("2026-09-03", 1),
				day("2026-09-04", 0), day("2026-09-05", 0), day("2026-09-06", 0),
			},
			today:   "2026-09-03",
			current: 3, longest: 3,
		},
		{
			// A zero on today is a day still in progress, not a broken streak.
			name: "quiet today keeps yesterday's run alive",
			days: []Day{
				day("2026-09-01", 4), day("2026-09-02", 4), day("2026-09-03", 0),
			},
			today:   "2026-09-03",
			current: 2, longest: 2,
		},
		{
			name: "a gap before today zeroes the current run",
			days: []Day{
				day("2026-08-29", 3), day("2026-08-30", 3), day("2026-08-31", 3),
				day("2026-09-01", 0), day("2026-09-02", 0), day("2026-09-03", 0),
			},
			today:   "2026-09-03",
			current: 0, longest: 3,
		},
		{
			name: "longest is remembered from earlier in the year",
			days: []Day{
				day("2026-08-01", 1), day("2026-08-02", 1), day("2026-08-03", 1),
				day("2026-08-04", 1), day("2026-08-05", 0),
				day("2026-09-02", 1), day("2026-09-03", 1),
			},
			today:   "2026-09-03",
			current: 2, longest: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current, longest := computeStreaks(tt.days, at(tt.today))
			if current != tt.current || longest != tt.longest {
				t.Errorf("computeStreaks = (%d, %d), want (%d, %d)",
					current, longest, tt.current, tt.longest)
			}
		})
	}
}

func TestUpToDropsFutureCells(t *testing.T) {
	days := []Day{
		day("2026-09-02", 1), day("2026-09-03", 2),
		day("2026-09-04", 0), day("2026-09-05", 0),
	}
	got := upTo(days, at("2026-09-03"))
	if len(got) != 2 {
		t.Fatalf("kept %d days, want 2", len(got))
	}
	if last := got[len(got)-1]; !last.Date.Equal(at("2026-09-03")) {
		t.Errorf("last kept day is %s, want 2026-09-03", last.Date.Format("2006-01-02"))
	}

	if got := upTo(days, at("2026-01-01")); got != nil {
		t.Errorf("a cutoff before the whole window should keep nothing, kept %d", len(got))
	}
}

func TestAggregateLanguages(t *testing.T) {
	t.Run("no bytes yields no rows", func(t *testing.T) {
		if got := aggregateLanguages(map[string]int{}); got != nil {
			t.Errorf("want nil, got %v", got)
		}
		if got := aggregateLanguages(map[string]int{"Go": 0}); got != nil {
			t.Errorf("a zero total must not produce rows, got %v", got)
		}
	})

	t.Run("sorted descending with shares summing to one", func(t *testing.T) {
		got := aggregateLanguages(map[string]int{"Go": 100, "Python": 300, "Shell": 100})
		if len(got) != 3 {
			t.Fatalf("got %d rows, want 3", len(got))
		}
		if got[0].Name != "Python" {
			t.Errorf("leader is %q, want Python", got[0].Name)
		}
		// Name is the tiebreaker, so equal byte counts have a stable order.
		if got[1].Name != "Go" || got[2].Name != "Shell" {
			t.Errorf("tie broken as %q,%q; want Go,Shell", got[1].Name, got[2].Name)
		}
		var sum float64
		for _, l := range got {
			sum += l.Share
		}
		if sum < 0.9999 || sum > 1.0001 {
			t.Errorf("shares sum to %v, want 1", sum)
		}
	})

	t.Run("the tail folds into one Other bucket", func(t *testing.T) {
		byLang := map[string]int{}
		for i, name := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"} {
			byLang[name] = 100 - i // strictly descending, so the fold is predictable
		}
		got := aggregateLanguages(byLang)
		if len(got) != maxLanguageSlots+1 {
			t.Fatalf("got %d rows, want %d", len(got), maxLanguageSlots+1)
		}
		last := got[len(got)-1]
		if last.Name != "Other" {
			t.Fatalf("last row is %q, want Other", last.Name)
		}
		// G+H+I = 94+93+92
		if last.Bytes != 279 {
			t.Errorf("Other holds %d bytes, want 279", last.Bytes)
		}
		var sum float64
		for _, l := range got {
			sum += l.Share
		}
		if sum < 0.9999 || sum > 1.0001 {
			t.Errorf("shares sum to %v after folding, want 1", sum)
		}
	})
}

func TestMaxCountFloorsAtOne(t *testing.T) {
	if got := maxCount([]Day{day("2026-09-01", 0), day("2026-09-02", 0)}); got != 1 {
		t.Errorf("a quiet window must floor at 1 so bar scaling never divides by zero, got %d", got)
	}
	if got := maxCount([]Day{day("2026-09-01", 3), day("2026-09-02", 7)}); got != 7 {
		t.Errorf("got %d, want 7", got)
	}
}

func TestLastDays(t *testing.T) {
	days := []Day{day("2026-09-01", 1), day("2026-09-02", 2), day("2026-09-03", 3)}
	if got := lastDays(days, 2); len(got) != 2 || got[0].Count != 2 {
		t.Errorf("lastDays(2) = %v", got)
	}
	if got := lastDays(days, 10); len(got) != 3 {
		t.Errorf("asking for more days than exist should return them all, got %d", len(got))
	}
}

func TestPushedWithin(t *testing.T) {
	now := at("2026-09-03")
	fresh := Repo{PushedAt: at("2026-08-20")}
	stale := Repo{PushedAt: at("2026-06-01")}
	if !fresh.PushedWithin(now, cityFreshWindow) {
		t.Error("a push 14 days ago should be inside the 30 day window")
	}
	if stale.PushedWithin(now, cityFreshWindow) {
		t.Error("a push in June should be outside the 30 day window")
	}
	if (Repo{}).PushedWithin(now, cityFreshWindow) {
		t.Error("a repo with no push date must not read as fresh")
	}
}
