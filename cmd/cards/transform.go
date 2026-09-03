package main

import (
	"sort"
	"time"
)

// toStats flattens the GraphQL reply into the render model.
//
// Private-repo activity only appears when the token belongs to the profile owner
// (a PAT with read:user); with the Actions-provided token
// restrictedContributionsCount comes back as 0 and the totals are public-only.
func (r *apiResponse) toStats(now time.Time) *Stats {
	u := r.Data.User
	c := u.Contributions

	s := &Stats{
		Login:              u.Login,
		Name:               u.Name,
		Location:           u.Location,
		Followers:          u.Followers.TotalCount,
		PublicRepos:        u.Repositories.TotalCount,
		Commits:            c.TotalCommitContributions + c.RestrictedContributionsCount,
		PullRequests:       c.TotalPullRequestContributions,
		Issues:             c.TotalIssueContributions,
		Reviews:            c.TotalPullRequestReviewContributions,
		TotalContributions: c.Calendar.TotalContributions + c.RestrictedContributionsCount,
		GeneratedAt:        now.UTC(),
	}

	byLang := map[string]int{}
	for _, node := range u.Repositories.Nodes {
		s.Stars += node.StargazerCount

		primary := ""
		if node.PrimaryLanguage != nil {
			primary = node.PrimaryLanguage.Name
		}
		pushed, err := time.Parse(time.RFC3339, node.PushedAt)
		if err != nil {
			pushed = time.Time{}
		}
		s.Repos = append(s.Repos, Repo{
			Name:     node.Name,
			Stars:    node.StargazerCount,
			PushedAt: pushed,
			Primary:  primary,
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
	s.CurrentStreak, s.LongestStreak = computeStreaks(s.Days)

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
