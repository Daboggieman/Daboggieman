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

const query = `query($login:String!){
  user(login:$login){
    login
    name
    location
    followers{totalCount}
    repositories(first:100, ownerAffiliations:OWNER, isFork:false, orderBy:{field:PUSHED_AT,direction:DESC}){
      totalCount
      nodes{
        name
        stargazerCount
        pushedAt
        diskUsage
        primaryLanguage{name}
        languages(first:12, orderBy:{field:SIZE,direction:DESC}){edges{size node{name}}}
        defaultBranchRef{target{... on Commit{history{totalCount}}}}
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
  }
}`

// apiResponse mirrors the shape of the GraphQL reply. Only the fields the cards
// consume are modelled.
type apiResponse struct {
	Data struct {
		User struct {
			Login     string `json:"login"`
			Name      string `json:"name"`
			Location  string `json:"location"`
			Followers struct {
				TotalCount int `json:"totalCount"`
			} `json:"followers"`
			Repositories struct {
				TotalCount int `json:"totalCount"`
				Nodes      []struct {
					Name            string `json:"name"`
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
					DefaultBranchRef *struct {
						Target *struct {
							History struct {
								TotalCount int `json:"totalCount"`
							} `json:"history"`
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
		"query":     query,
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
