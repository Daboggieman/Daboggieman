package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const endpoint = "https://api.github.com/graphql"

const queryTemplate = `query($login:String!){
  user(login:$login){
    login
    name
    location
    createdAt
    followers{totalCount}
    repositories(first:100, ownerAffiliations:OWNER, isFork:false, orderBy:{field:PUSHED_AT,direction:DESC}){
      totalCount
      nodes{
        name
        isPrivate
        stargazerCount
        pushedAt
        diskUsage
        primaryLanguage{name}
        languages(first:12, orderBy:{field:SIZE,direction:DESC}){edges{size node{name}}}
        defaultBranchRef{target{... on Commit{
          history{totalCount}
          recent: history(first:50){nodes{committedDate author{user{login}}}}
        }}}
      }
    }
    contributionsCollection{
      totalCommitContributions
      totalPullRequestContributions
      totalIssueContributions
      totalPullRequestReviewContributions
      restrictedContributionsCount
      contributionCalendar{
        totalContributions
        weeks{contributionDays{date contributionCount}}
      }
    }
%s
  }
}`

// yearSpan is how many calendar years the year grid reaches back, counting the
// current one. Six fits a decade-shaped career on a card a README can render
// without shrinking the labels, and years before the account existed come back
// empty and are trimmed.
const yearSpan = 6

// buildQuery fills in the per-year aliases. contributionsCollection is capped at
// one year per call by the API, so the only way to get a decade in one round trip
// is to ask for each year under its own alias.
func buildQuery(now time.Time) string {
	var b strings.Builder
	for i := 0; i < yearSpan; i++ {
		y := now.UTC().Year() - i
		// The window is the whole calendar year, clamped to now for the current one:
		// asking for a future range is a hard error rather than an empty answer.
		to := time.Date(y, 12, 31, 23, 59, 59, 0, time.UTC)
		if to.After(now.UTC()) {
			to = now.UTC()
		}
		fmt.Fprintf(&b, "\n    y%d: contributionsCollection(from:%q, to:%q){\n"+
			"      contributionCalendar{totalContributions weeks{contributionDays{date contributionCount}}}\n    }",
			i, time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339), to.Format(time.RFC3339))
	}
	return fmt.Sprintf(queryTemplate, b.String())
}

// yearCalendar is one aliased year's calendar.
type yearCalendar struct {
	Calendar struct {
		TotalContributions int `json:"totalContributions"`
		Weeks              []struct {
			ContributionDays []struct {
				Date              string `json:"date"`
				ContributionCount int    `json:"contributionCount"`
			} `json:"contributionDays"`
		} `json:"weeks"`
	} `json:"contributionCalendar"`
}

// apiResponse mirrors the shape of the GraphQL reply. Only the fields the cards
// consume are modelled.
type apiResponse struct {
	Data struct {
		User struct {
			Login     string `json:"login"`
			Name      string `json:"name"`
			Location  string `json:"location"`
			CreatedAt string `json:"createdAt"`
			Followers struct {
				TotalCount int `json:"totalCount"`
			} `json:"followers"`
			Repositories struct {
				TotalCount int `json:"totalCount"`
				Nodes      []struct {
					Name            string `json:"name"`
					IsPrivate       bool   `json:"isPrivate"`
					StargazerCount  int    `json:"stargazerCount"`
					PushedAt        string `json:"pushedAt"`
					DiskUsage       int    `json:"diskUsage"`
					PrimaryLanguage *struct {
						Name string `json:"name"`
					} `json:"primaryLanguage"`
					Languages struct {
						Edges []struct {
							Size int `json:"size"`
							Node struct {
								Name string `json:"name"`
							} `json:"node"`
						} `json:"edges"`
					} `json:"languages"`
					// defaultBranchRef is null on an empty repo, and target only
					// carries history when it is a Commit, so both hops are optional.
					//
					// recent is a second, aliased pass over the same history: the
					// totalCount is the building's height, and these fifty timestamps
					// are the sample the rhythm card reads. Fifty per repo keeps the
					// query's node count in the low thousands while still covering
					// months of work on anything but the busiest repo.
					DefaultBranchRef *struct {
						Target *struct {
							History struct {
								TotalCount int `json:"totalCount"`
							} `json:"history"`
							Recent struct {
								Nodes []struct {
									CommittedDate string `json:"committedDate"`
									Author        *struct {
										User *struct {
											Login string `json:"login"`
										} `json:"user"`
									} `json:"author"`
								} `json:"nodes"`
							} `json:"recent"`
						} `json:"target"`
					} `json:"defaultBranchRef"`
				} `json:"nodes"`
			} `json:"repositories"`
			Contributions struct {
				TotalCommitContributions            int `json:"totalCommitContributions"`
				TotalPullRequestContributions       int `json:"totalPullRequestContributions"`
				TotalIssueContributions             int `json:"totalIssueContributions"`
				TotalPullRequestReviewContributions int `json:"totalPullRequestReviewContributions"`
				RestrictedContributionsCount        int `json:"restrictedContributionsCount"`
				Calendar                            struct {
					TotalContributions int `json:"totalContributions"`
					Weeks              []struct {
						ContributionDays []struct {
							Date              string `json:"date"`
							ContributionCount int    `json:"contributionCount"`
						} `json:"contributionDays"`
					} `json:"weeks"`
				} `json:"contributionCalendar"`
			} `json:"contributionsCollection"`
			// One field per alias rather than a map, so the decode stays static and
			// a missing year is a zero value instead of a lookup that has to be
			// guarded at every use.
			Y0 yearCalendar `json:"y0"`
			Y1 yearCalendar `json:"y1"`
			Y2 yearCalendar `json:"y2"`
			Y3 yearCalendar `json:"y3"`
			Y4 yearCalendar `json:"y4"`
			Y5 yearCalendar `json:"y5"`
		} `json:"user"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// fetch runs the profile query and returns the raw response body alongside the
// decoded result, so callers can persist a fixture for offline rendering.
func fetch(ctx context.Context, login, token string) (*apiResponse, []byte, error) {
	body, err := json.Marshal(map[string]any{
		"query":     buildQuery(time.Now()),
		"variables": map[string]string{"login": login},
	})
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Daboggieman-profile-cards")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, raw, fmt.Errorf("graphql http %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	out, err := decode(raw)
	if err != nil {
		return nil, raw, err
	}
	return out, raw, nil
}

func decode(raw []byte) (*apiResponse, error) {
	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode graphql response: %w", err)
	}
	if len(out.Errors) > 0 {
		msgs := make([]string, 0, len(out.Errors))
		for _, e := range out.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("graphql errors: %s", strings.Join(msgs, "; "))
	}
	if out.Data.User.Login == "" {
		return nil, fmt.Errorf("graphql response contained no user")
	}
	return &out, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
