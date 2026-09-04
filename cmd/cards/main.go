// Command cards renders the SVG cards on this profile from live GitHub data.
//
// The cards are generated here, committed into this repo, and served from it, so
// nothing on the profile depends on a third-party widget host staying up or
// staying paid.
//
//	# live; read:user is enough for public data, and the repo scope is what
//	# makes private repositories show up as well
//	GITHUB_TOKEN=... go run ./cmd/cards
//
//	# keep named repos off the profile entirely, private or not
//	GITHUB_TOKEN=... go run ./cmd/cards -exclude secret-client-work,old-experiment
//
//	# offline, from a saved response, touching nothing that is tracked
//	go run ./cmd/cards -fixture testdata/profile.json -out /tmp/cards -readme "" -history ""
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	log.SetFlags(0)
	login := flag.String("login", "Daboggieman", "GitHub login to render")
	out := flag.String("out", "assets", "directory to write the SVG cards into")
	fixture := flag.String("fixture", "", "render from a saved GraphQL response instead of the network")
	dump := flag.String("dump", "", "also write the live GraphQL response to this path")
	readme := flag.String("readme", "README.md", "README to refresh the table view in (empty to skip)")
	exclude := flag.String("exclude", "", "comma-separated repo names to leave out of every card")
	buildings := flag.Int("buildings", maxCityBlocks, "how many repos the skyline draws; the rest stay in the table view")
	history := flag.String("history", "history.jsonl", "archive of daily totals, the only source of a delta (empty to skip)")
	message := flag.String("message", "", "write a commit subject naming what changed to this path")
	tz := flag.Int("tz", 0, "hours to shift commit stamps that arrived without an offset (see the rhythm card)")
	flag.Parse()

	if *buildings > 0 {
		maxCityBlocks = *buildings
	}
	tzFallback = *tz
	if err := run(*login, *out, *fixture, *dump, *readme, *exclude, *history, *message); err != nil {
		log.Fatalf("cards: %v", err)
	}
}

func run(login, out, fixture, dump, readme, exclude, history, message string) error {
	var (
		resp *apiResponse
		raw  []byte
		err  error
	)

	if fixture != "" {
		raw, err = os.ReadFile(fixture)
		if err != nil {
			return fmt.Errorf("read fixture: %w", err)
		}
		if resp, err = decode(raw); err != nil {
			return err
		}
	} else {
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			return fmt.Errorf("GITHUB_TOKEN is not set (or pass -fixture for an offline render)")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if resp, raw, err = fetch(ctx, login, token); err != nil {
			return err
		}
	}

	if dump != "" {
		if err := os.WriteFile(dump, raw, 0o644); err != nil {
			return fmt.Errorf("write dump: %w", err)
		}
	}

	stats := resp.toStats(time.Now(), parseExclude(exclude))

	// The archive is read before anything renders, because the cards ask it for
	// their baselines, and written after, so a render that fails does not leave a
	// reading behind for a day whose cards were never produced.
	if history != "" {
		if stats.History, err = loadHistory(history); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	cards := map[string]string{
		"terminal.svg":  renderTerminal(stats),
		"languages.svg": renderLanguages(stats),
		"activity.svg":  renderActivity(stats),
		"collab.svg":    renderCollab(stats),
		"calendar.svg":  renderHeatmap(stats),
		"rhythm.svg":    renderRhythm(stats),
		"years.svg":     renderYears(stats),
		"drift.svg":     renderDrift(stats),
		"city.svg":      renderCity(stats),
	}
	for name, svg := range cards {
		path := filepath.Join(out, name)
		if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		log.Printf("wrote %s (%d bytes)", path, len(svg))
	}

	if readme != "" {
		changed, err := updateReadme(readme, stats)
		if err != nil {
			return err
		}
		if changed {
			log.Printf("refreshed the table view in %s", readme)
		}
	}

	// The subject is computed against the archive as it was *before* today's reading
	// lands in it, so it describes the change rather than comparing today with
	// itself.
	subject := commitSubject(stats)
	if message != "" {
		if err := os.WriteFile(message, []byte(subject+"\n"), 0o644); err != nil {
			return fmt.Errorf("write commit subject: %w", err)
		}
	}
	if history != "" {
		today := stats.Snapshot()
		archive := mergeSnapshot(stats.History, today)
		if err := writeHistory(history, archive); err != nil {
			return err
		}
		log.Printf("archived %s in %s (%s)", today.Date, history, plural(len(archive), "reading"))
	}
	log.Printf("%s", subject)

	log.Printf("%s: %d contributions, %d day streak (best %d), %d languages, %d of %d repos private",
		stats.Login, stats.TotalContributions, stats.CurrentStreak, stats.LongestStreak,
		len(stats.Languages), stats.PrivateCount(), stats.RepoCount)
	return nil
}

// parseExclude turns the -exclude list into a lookup. Matching is case-insensitive
// because nobody types a repo name back with its original capitalisation.
func parseExclude(list string) map[string]bool {
	out := map[string]bool{}
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out[strings.ToLower(name)] = true
		}
	}
	return out
}
