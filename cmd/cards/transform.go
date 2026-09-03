package main

import (
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

// CityBlocks ranks repos by commit volume for the skyline, tallest first, and
// caps the count at maxCityBlocks so the buildings never get too narrow to label.
// Repos with no commits on the default branch have nothing to build with and are
// dropped rather than drawn as slivers.
func (s *Stats) CityBlocks() []Repo {
	out := make([]Repo, 0, len(s.Repos))
	for _, r := range s.Repos {
		if r.Commits <= 0 {
			continue
		}
		out = append(out, r)
	}
	// Name breaks ties so the skyline order — and therefore the committed SVG —
	// is stable between runs.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commits != out[j].Commits {
			return out[i].Commits > out[j].Commits
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > maxCityBlocks {
		out = out[:maxCityBlocks]
	}
	return out
}
