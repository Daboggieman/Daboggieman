package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// toStats flattens the GraphQL reply into the render model.
//
// How much of the profile is visible is entirely a property of the token. The
// Actions-provided GITHUB_TOKEN sees public repos only, and
// restrictedContributionsCount comes back as 0. A PAT with the repo scope returns
// private repositories as ordinary nodes and folds their contributions into the
// calendar, so the same code renders a fuller profile with no changes here.
//
// hide drops repos by name before anything is counted, so an excluded repo is
// absent from the skyline, the language shares, the star total and the table —
// not merely unlinked. It is the escape hatch for private work that should not be
// named on a public page at all.
func (r *apiResponse) toStats(now time.Time, hide map[string]bool) *Stats {
	u := r.Data.User
	c := u.Contributions

	s := &Stats{
		Login:              u.Login,
		Name:               u.Name,
		Location:           u.Location,
		Followers:          u.Followers.TotalCount,
		Commits:            c.TotalCommitContributions + c.RestrictedContributionsCount,
		PullRequests:       c.TotalPullRequestContributions,
		Issues:             c.TotalIssueContributions,
		Reviews:            c.TotalPullRequestReviewContributions,
		TotalContributions: c.Calendar.TotalContributions + c.RestrictedContributionsCount,
		GeneratedAt:        now.UTC(),
		CreatedAt:          parseDay(u.CreatedAt),
		Years:              yearRows([yearSpan]yearCalendar{u.Y0, u.Y1, u.Y2, u.Y3, u.Y4, u.Y5}, now.UTC()),
	}

	byLang := map[string]int{}
	for _, node := range u.Repositories.Nodes {
		if hide[strings.ToLower(node.Name)] {
			continue
		}
		s.RepoCount++
		s.Stars += node.StargazerCount

		primary := ""
		if node.PrimaryLanguage != nil {
			primary = node.PrimaryLanguage.Name
		}
		pushed, err := time.Parse(time.RFC3339, node.PushedAt)
		if err != nil {
			pushed = time.Time{}
		}
		commits := 0
		if br := node.DefaultBranchRef; br != nil && br.Target != nil {
			commits = br.Target.History.TotalCount
			// Attribution matters more than sample size here: a repo with other
			// contributors would otherwise lend their working hours to this card.
			// A commit whose author GitHub cannot match to an account is dropped
			// rather than guessed at, and the card reports the sample it got.
			for _, cm := range br.Target.Recent.Nodes {
				if cm.Author == nil || cm.Author.User == nil {
					continue
				}
				if !strings.EqualFold(cm.Author.User.Login, u.Login) {
					continue
				}
				at, err := time.Parse(time.RFC3339, cm.CommittedDate)
				if err != nil {
					continue
				}
				_, offset := at.Zone()
				s.CommitStamps = append(s.CommitStamps, CommitStamp{At: at, Offset: offset})
			}
		}
		s.Repos = append(s.Repos, Repo{
			Name:     node.Name,
			Stars:    node.StargazerCount,
			PushedAt: pushed,
			Primary:  primary,
			Commits:  commits,
			SizeKB:   node.DiskUsage,
			Private:  node.IsPrivate,
		})

		for _, e := range node.Languages.Edges {
			if e.Node.Name == "" || e.Size <= 0 {
				continue
			}
			byLang[e.Node.Name] += e.Size
		}
	}

	s.Languages = aggregateLanguages(byLang)
	// Oldest first, so the committed SVG is stable whatever order the repos came
	// back in.
	sort.Slice(s.CommitStamps, func(i, j int) bool {
		if !s.CommitStamps[i].At.Equal(s.CommitStamps[j].At) {
			return s.CommitStamps[i].At.Before(s.CommitStamps[j].At)
		}
		return s.CommitStamps[i].Offset < s.CommitStamps[j].Offset
	})

	for _, w := range c.Calendar.Weeks {
		for _, d := range w.ContributionDays {
			day, err := time.Parse("2006-01-02", d.Date)
			if err != nil {
				continue
			}
			s.Days = append(s.Days, Day{Date: day, Count: d.ContributionCount})
		}
	}
	// The API returns weeks in order, but sorting makes the streak walk
	// independent of that guarantee.
	sort.Slice(s.Days, func(i, j int) bool { return s.Days[i].Date.Before(s.Days[j].Date) })
	// The calendar is week-aligned and so runs past today. Trimming here keeps the
	// sparkline's trailing day on the real today rather than on an empty future cell.
	s.Days = upTo(s.Days, now)
	s.CurrentStreak, s.LongestStreak = computeStreaks(s.Days, now)

	return s
}

// TopRepo returns the most recently pushed repository, skipping the profile
// repo itself since it is not a project.
func (s *Stats) TopRepo(skip string) (Repo, bool) {
	best := Repo{}
	found := false
	for _, r := range s.Repos {
		if r.Name == skip {
			continue
		}
		if !found || r.PushedAt.After(best.PushedAt) {
			best, found = r, true
		}
	}
	return best, found
}

// TopLanguageNames returns up to n language names, excluding the folded
// "Other" bucket.
func (s *Stats) TopLanguageNames(limit int) []string {
	out := []string{}
	for _, l := range s.Languages {
		if l.Name == "Other" {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, l.Name)
	}
	return out
}

// RankedRepos is every repo with something to show, most commits first. Repos with
// no commits on the default branch — empty ones, and ones with no default branch
// at all — have no height to draw and no activity to report, so they are dropped.
//
// This is the complete list. Only the drawing is capped; the table view walks this
// so a repo is never missing from the page just because it did not fit the street.
func (s *Stats) RankedRepos() []Repo {
	out := make([]Repo, 0, len(s.Repos))
	for _, r := range s.Repos {
		if r.Commits <= 0 {
			continue
		}
		out = append(out, r)
	}
	// Name breaks ties so the order — and therefore the committed SVG — is stable
	// between runs.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commits != out[j].Commits {
			return out[i].Commits > out[j].Commits
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// CityBlocks is the part of RankedRepos that actually gets built, capped at
// maxCityBlocks. The cap is a legibility floor, not a data decision: past it each
// building is narrower than its own name, and an unlabelled building is a
// rectangle. Whatever the cap cuts still appears in the table view, and the card
// says how many it left out.
func (s *Stats) CityBlocks() []Repo {
	out := s.RankedRepos()
	if len(out) > maxCityBlocks {
		out = out[:maxCityBlocks]
	}
	return out
}

// parseDay reads an API timestamp, returning the zero time on anything it cannot
// parse. A missing createdAt costs the year grid its "record starts" note and
// nothing else, so it is not worth failing the render over.
func parseDay(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// yearRows bins each aliased year's calendar into months.
//
// Years before the account existed come back empty, and an empty row drawn as a
// full-width band of "quiet" months is a lie about a year that had not started
// yet — so leading empty years are trimmed and the card says where the record
// begins.
func yearRows(blocks [yearSpan]yearCalendar, now time.Time) []YearRow {
	rows := make([]YearRow, 0, yearSpan)
	for i := yearSpan - 1; i >= 0; i-- { // oldest first
		row := YearRow{Year: now.Year() - i}
		for _, wk := range blocks[i].Calendar.Weeks {
			for _, d := range wk.ContributionDays {
				day := parseDay(d.Date)
				if day.IsZero() || day.Year() != row.Year || d.ContributionCount == 0 {
					continue
				}
				row.Months[int(day.Month())-1] += d.ContributionCount
				row.Total += d.ContributionCount
				row.Active++
			}
		}
		if row.Total == 0 && len(rows) == 0 {
			continue // the account did not exist yet
		}
		rows = append(rows, row)
	}
	return rows
}

// ThisWeek is contributions over the trailing seven days of the calendar. Every
// other figure on the profile is a year or a lifetime; this is the only window
// short enough to describe what is happening now rather than what has happened.
func (s *Stats) ThisWeek() int {
	if len(s.Days) == 0 {
		return 0
	}
	cut := s.Days[len(s.Days)-1].Date.AddDate(0, 0, -6)
	n := 0
	for _, d := range s.Days {
		if !d.Date.Before(cut) {
			n += d.Count
		}
	}
	return n
}

// Focus is the current-work line: the repo pushed most recently, what it is
// written in, when it moved, and how much has landed in the last seven days.
//
// The profile repo is skipped by name. It is pushed every day by the workflow that
// renders these cards, so leaving it in would make the answer to "what am I working
// on" permanently "the thing that writes this card", which is true and useless.
func (s *Stats) Focus() (string, bool) {
	r, ok := s.TopRepo(s.Login)
	if !ok || r.PushedAt.IsZero() {
		return "", false
	}
	lang := r.Primary
	if lang == "" {
		lang = "no language detected"
	}
	return fmt.Sprintf("%s · %s · pushed %s · %s this week",
		r.Name, lang, agoWords(s.GeneratedAt, r.PushedAt),
		plural(s.ThisWeek(), "contribution")), true
}
