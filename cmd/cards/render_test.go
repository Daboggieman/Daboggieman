package main

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"
	"time"
)

// renderedAt pins the clock so the fixture's "occupied vs dormant" split and the
// streak walk are the same on every run. The fixture's newest push is
// 2026-09-03, so this is the date the data was captured for.
var renderedAt = time.Date(2026, 9, 3, 18, 0, 0, 0, time.UTC)

func loadFixture(t *testing.T) *Stats {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/profile.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	resp, err := decode(raw)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return resp.toStats(renderedAt, nil)
}

func TestFixtureDecodesIntoAWholeModel(t *testing.T) {
	s := loadFixture(t)

	if s.Login != "Daboggieman" {
		t.Errorf("login is %q", s.Login)
	}
	if len(s.Days) == 0 {
		t.Fatal("no calendar days survived the transform")
	}
	// toStats trims the week-aligned calendar, so the last cell must be a real
	// day at or before the render date, never an empty future one.
	if last := s.Days[len(s.Days)-1].Date; last.After(renderedAt) {
		t.Errorf("calendar still runs to %s, past the render date", last.Format("2006-01-02"))
	}
	if s.CurrentStreak == 0 {
		t.Error("the fixture has an active streak; a zero here is the week-alignment bug")
	}
	if s.LongestStreak < s.CurrentStreak {
		t.Errorf("longest (%d) cannot be under current (%d)", s.LongestStreak, s.CurrentStreak)
	}
	if len(s.Languages) == 0 {
		t.Error("no languages aggregated")
	}
	for _, r := range s.Repos {
		if r.Commits <= 0 {
			t.Errorf("%s decoded with %d commits; the defaultBranchRef hop is not wired up",
				r.Name, r.Commits)
		}
		if r.SizeKB <= 0 {
			t.Errorf("%s decoded with no diskUsage", r.Name)
		}
	}
}

func TestCityBlocksAreRankedAndCapped(t *testing.T) {
	s := loadFixture(t)
	blocks := s.CityBlocks()

	if len(blocks) == 0 {
		t.Fatal("no buildings to draw")
	}
	if len(blocks) > maxCityBlocks {
		t.Errorf("got %d buildings, over the %d cap", len(blocks), maxCityBlocks)
	}
	for i := 1; i < len(blocks); i++ {
		if blocks[i-1].Commits < blocks[i].Commits {
			t.Errorf("skyline is not tallest-first at %d: %s(%d) before %s(%d)",
				i, blocks[i-1].Name, blocks[i-1].Commits, blocks[i].Name, blocks[i].Commits)
		}
	}

	// A repo with no commits has no building to draw and must be dropped rather
	// than rendered as a sliver.
	empty := &Stats{Repos: []Repo{{Name: "fresh", Commits: 0}, {Name: "real", Commits: 5}}}
	got := empty.CityBlocks()
	if len(got) != 1 || got[0].Name != "real" {
		t.Errorf("commitless repos should be dropped, got %v", got)
	}
}

// cards is every renderer, so the shared invariants below are checked once each.
func cards(s *Stats) map[string]string {
	return map[string]string{
		"terminal":  renderTerminal(s),
		"languages": renderLanguages(s),
		"activity":  renderActivity(s),
		"collab":    renderCollab(s),
		"calendar":  renderHeatmap(s),
		"rhythm":    renderRhythm(s),
		"city":      renderCity(s),
	}
}

func TestEveryCardIsWellFormedXML(t *testing.T) {
	for name, svg := range cards(loadFixture(t)) {
		t.Run(name, func(t *testing.T) {
			d := xml.NewDecoder(strings.NewReader(svg))
			for {
				_, err := d.Token()
				if err != nil {
					if err.Error() == "EOF" {
						return
					}
					t.Fatalf("malformed SVG: %v", err)
				}
			}
		})
	}
}

func TestEveryCardIsSafeForGitHubsImageProxy(t *testing.T) {
	for name, svg := range cards(loadFixture(t)) {
		t.Run(name, func(t *testing.T) {
			// GitHub strips <script> and camo will not follow external refs, so a
			// card that needs either is a card that renders wrong on the profile.
			for _, banned := range []string{"<script", "<image", "xlink:href", "<foreignObject", "url(http", "@import"} {
				if strings.Contains(svg, banned) {
					t.Errorf("card contains %q, which will not survive the image proxy", banned)
				}
			}
			// role="img" plus a title and desc is the whole accessibility story
			// for an SVG served as an image.
			for _, required := range []string{`role="img"`, `<title id="t">`, `<desc id="d">`} {
				if !strings.Contains(svg, required) {
					t.Errorf("card is missing %s", required)
				}
			}
		})
	}
}

func TestAnimatedCardsHonourReducedMotion(t *testing.T) {
	for name, svg := range cards(loadFixture(t)) {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(svg, "animation:") {
				return // nothing moves on this card
			}
			if !strings.Contains(svg, "prefers-reduced-motion") {
				t.Error("card animates without a reduced-motion escape")
			}
			// The blank-card bug: a keyframe whose opening stop hides the mark
			// renders an empty card wherever the animation does not advance. Only
			// the opening stop matters — a cursor blink that ends at opacity:0 is
			// correct, one that starts there is the bug.
			for _, kf := range openingStops(svg) {
				for _, unsafe := range []string{"scaleX(0)", "scaleY(0)", "scale(0)", "opacity:0"} {
					if strings.Contains(kf.decls, unsafe) {
						t.Errorf("@keyframes %s opens on %q, which can render the card blank",
							kf.name, kf.decls)
					}
				}
			}
		})
	}
}

type keyframeStop struct{ name, decls string }

// openingStops pulls the time-zero stops out of every @keyframes block in the
// card's stylesheet — the frame a renderer draws when it never advances the
// animation. Stop order inside a block is not significant in CSS, so every stop
// is examined and the ones anchored at 0% are returned.
func openingStops(svg string) []keyframeStop {
	var out []keyframeStop
	for _, tail := range strings.Split(svg, "@keyframes ")[1:] {
		name, body, ok := strings.Cut(tail, "{")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		// The block ends where its nesting returns to zero.
		depth, end := 1, -1
		for i, r := range body {
			if r == '{' {
				depth++
			} else if r == '}' {
				if depth--; depth == 0 {
					end = i
					break
				}
			}
		}
		if end < 0 {
			continue
		}
		for _, stop := range strings.Split(body[:end], "}") {
			sel, decls, ok := strings.Cut(stop, "{")
			if !ok {
				continue
			}
			// Only a stop that actually renders at time zero can blank the card.
			if s := strings.TrimSpace(sel); s != "from" && !strings.HasPrefix(s, "0%") {
				continue
			}
			out = append(out, keyframeStop{name: name, decls: strings.TrimSpace(decls)})
		}
	}
	return out
}

func TestOpeningStopsFindsTheTimeZeroFrame(t *testing.T) {
	// The parser is the thing standing between a blank card and a green build, so
	// it gets its own cases: order inside a block does not matter, and a stop that
	// only hides the mark on the way out is not a defect.
	got := openingStops(`@keyframes a{from{opacity:.3}to{opacity:1}}` +
		`@keyframes b{to{opacity:1}0%{opacity:0}}` +
		`@keyframes c{0%,49%{opacity:1}50%,100%{opacity:0}}`)
	if len(got) != 3 {
		t.Fatalf("found %d opening stops, want 3: %v", len(got), got)
	}
	if got[0].decls != "opacity:.3" || got[1].name != "b" || got[1].decls != "opacity:0" {
		t.Errorf("parsed %v", got)
	}
	if got[2].decls != "opacity:1" {
		t.Errorf("the blink's opening stop is %q; the opacity:0 stop is at 50%%", got[2].decls)
	}
}

func TestTerminalCardCarriesItsNumbers(t *testing.T) {
	s := loadFixture(t)
	svg := renderTerminal(s)

	for _, want := range []string{
		"raphel@github:~$",
		commas(s.Commits) + " commits",
		"current vs personal best",
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("terminal card is missing %q", want)
		}
	}
	// The meter's fill must be its real value, not a placeholder: a card that
	// draws the fill at the full track width silently reads 100%.
	if s.CurrentStreak >= s.LongestStreak {
		t.Skip("fixture streak is at its personal best, so there is no partial fill to check")
	}
	if !strings.Contains(svg, `fill="`+trackGreen+`"`) {
		t.Error("meter has no unfilled track, so the remaining distance is invisible")
	}
}

func TestCityCardNamesAndValuesEveryBuilding(t *testing.T) {
	s := loadFixture(t)
	svg := renderCity(s)

	for _, r := range s.CityBlocks() {
		// Truncated names only survive in the tooltip, which is where the test
		// looks: identity must never depend on reading the 3D shape.
		if !strings.Contains(svg, esc(r.Name)) {
			t.Errorf("city card never names %s", r.Name)
		}
		if !strings.Contains(svg, commas(r.Commits)) {
			t.Errorf("city card never prints %s's commit count", r.Name)
		}
	}
	// Two facade states means a legend is required, not optional.
	for _, want := range []string{"occupied", "dormant", "height = commits"} {
		if !strings.Contains(svg, want) {
			t.Errorf("city card is missing %q", want)
		}
	}
}

func TestCardsRenderAnEmptyProfileWithoutPanicking(t *testing.T) {
	empty := &Stats{Login: "nobody", GeneratedAt: renderedAt}
	for name, svg := range cards(empty) {
		if svg == "" {
			t.Errorf("%s rendered nothing at all", name)
		}
		if !strings.Contains(svg, "<svg") {
			t.Errorf("%s did not produce an SVG", name)
		}
	}
}

func TestTableViewMirrorsThePlottedValues(t *testing.T) {
	s := loadFixture(t)
	table := renderTableView(s)

	// The table is the accessible twin of the cards, so every plotted series has
	// to appear in it.
	for _, l := range s.Languages {
		if !strings.Contains(table, l.Name) {
			t.Errorf("table view omits the language %s", l.Name)
		}
	}
	for _, r := range s.CityBlocks() {
		if !strings.Contains(table, r.Name) {
			t.Errorf("table view omits the building %s", r.Name)
		}
	}
	for _, want := range []string{"Current streak", "Longest streak", "Stars earned", "Building"} {
		if !strings.Contains(table, want) {
			t.Errorf("table view is missing the %q row", want)
		}
	}
}

func TestUpdateReadmeOwnsOnlyTheMarkedBlock(t *testing.T) {
	s := loadFixture(t)
	dir := t.TempDir()
	path := dir + "/README.md"

	original := "# Title\n\nhand written\n\n" + tableStart + "\nstale\n" + tableEnd + "\n\nalso hand written\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := updateReadme(path, s)
	if err != nil {
		t.Fatalf("updateReadme: %v", err)
	}
	if !changed {
		t.Fatal("a stale block should have been rewritten")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	if !strings.Contains(body, "hand written") || !strings.Contains(body, "also hand written") {
		t.Error("the generator wrote outside its markers")
	}
	if strings.Contains(body, "stale") {
		t.Error("the old block survived")
	}

	// A second pass over identical data must report no change, or the workflow
	// commits a no-op every single run.
	changed, err = updateReadme(path, s)
	if err != nil {
		t.Fatalf("second updateReadme: %v", err)
	}
	if changed {
		t.Error("re-rendering the same data reported a change")
	}
}

func TestUpdateReadmeRejectsBrokenMarkers(t *testing.T) {
	s := loadFixture(t)
	dir := t.TempDir()

	cases := map[string]string{
		"unpaired": "# Title\n" + tableStart + "\nno end\n",
		"reversed": "# Title\n" + tableEnd + "\nbackwards\n" + tableStart + "\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := dir + "/" + name + ".md"
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := updateReadme(path, s); err == nil {
				t.Error("want an error, got none")
			}
		})
	}

	// A README with no markers at all is not an error; there is simply nothing
	// to refresh.
	path := dir + "/plain.md"
	if err := os.WriteFile(path, []byte("# Just a title\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := updateReadme(path, s)
	if err != nil {
		t.Errorf("an unmarked README should be left alone, got %v", err)
	}
	if changed {
		t.Error("an unmarked README should not be rewritten")
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	// The workflow commits whatever this produces, so two renders of the same
	// data must be byte-identical or every scheduled run churns the repo.
	a, b := cards(loadFixture(t)), cards(loadFixture(t))
	for name := range a {
		if a[name] != b[name] {
			t.Errorf("%s is not deterministic across renders", name)
		}
	}
}

func TestPrivateReposAreCountedButNeverLinked(t *testing.T) {
	s := loadFixture(t)

	if s.PrivateCount() == 0 {
		t.Fatal("the fixture has no private repo, so the unlinked path is untested")
	}
	var priv Repo
	for _, r := range s.CityBlocks() {
		if r.Private {
			priv = r
			break
		}
	}
	if priv.Name == "" {
		t.Fatal("no private repo reached the skyline")
	}

	// The whole contract in two assertions: the activity is published, the door
	// stays shut. A link to a private repo is a 404 for every visitor.
	table := renderTableView(s)
	if !strings.Contains(table, priv.Name) || !strings.Contains(table, commas(priv.Commits)) {
		t.Errorf("table view omits private repo %s or its commit count", priv.Name)
	}
	link := "https://github.com/" + s.Login + "/" + priv.Name
	if strings.Contains(table, link) {
		t.Errorf("table view links %s, which 404s for visitors", link)
	}
	if !strings.Contains(table, "| private |") {
		t.Error("table view never marks a row private")
	}

	// On the card the state has to be readable as words, not inferred from a
	// missing link that an SVG does not even have.
	svg := renderCity(s)
	if !strings.Contains(svg, "private") {
		t.Error("city card never says which buildings are private")
	}
}

func TestExcludeKeepsARepoOutOfEverything(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/profile.json")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}

	full := resp.toStats(renderedAt, nil)
	drop := full.CityBlocks()[0].Name // the tallest building, so the effect is visible
	got := resp.toStats(renderedAt, map[string]bool{strings.ToLower(drop): true})

	if got.RepoCount != full.RepoCount-1 {
		t.Errorf("repo count went %d -> %d, want one fewer", full.RepoCount, got.RepoCount)
	}
	for _, r := range got.Repos {
		if r.Name == drop {
			t.Fatalf("%s survived the exclude list", drop)
		}
	}
	// Excluding has to be total, not cosmetic: an excluded repo must not leak
	// through the table view or the card either.
	if strings.Contains(renderTableView(got), drop) {
		t.Errorf("%s still appears in the table view", drop)
	}
	if strings.Contains(renderCity(got), esc(drop)) {
		t.Errorf("%s still appears on the city card", drop)
	}
}
