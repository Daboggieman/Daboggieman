package main

import (
	"fmt"
	"math"
	"time"
)

// The city card: each owned repo is one building on an axonometric street.
//
// Form. Underneath the perspective this is a ranked bar chart — one quantitative
// value (commits) across nominal categories (repos), so bars scaled to the
// leader, sorted tallest-first, every bar directly labelled with its own value.
// The 3D is a skin on that chart, not an extra encoding, which is why nothing
// here depends on reading a depth cue.
//
// Encodings, one measure per channel:
//
//	height     commits on the default branch   (the primary read)
//	footprint  repo size on disk               (secondary, sqrt-compressed)
//	lit / dark pushed inside cityFreshWindow   (state, also carried by a glyph)
//	star badge stargazers, only when non-zero
//
// Stars deliberately do not drive width. Across this profile stars run 0..3, so a
// width encoding would collapse to "all buildings identical" and the caption would
// promise a reading the picture cannot deliver; as a badge over the roof the same
// number is legible at a glance. Repo size always varies, so it gets the footprint.
//
// The three faces of a building are a lightness ramp of one hue — a lighting
// model, not a data scale. Occupied and dormant are the only two categories, they
// carry a legend and a per-building glyph, and their fills differ in lightness as
// well as hue, so the state survives any colour vision and greyscale print.

// facade is the three-face lightness ramp for one building state.
type facade struct {
	top    string // lit from above, so the lightest step
	front  string
	side   string // turned away, so the darkest step
	window string
	label  string // the ink the name wears
	glyph  string // redundant, non-colour marker for the state
}

var (
	// Occupied: the accent's own ramp, roof brightest.
	facadeLive = facade{
		top:    accent,
		front:  accentDim,
		side:   trackGreen,
		window: "#7EE787", // two steps up the ramp, so windows read as lit
		label:  inkHi,
		glyph:  "●",
	}
	// Dormant: the same shading logic in neutrals, windows darker than the wall.
	facadeDark = facade{
		top:    deEmph,
		front:  baseline,
		side:   gridline,
		window: surface,
		label:  inkMid,
		glyph:  "○",
	}
)

// cityRow is one street: a contiguous slice of the ranked list plus the ground
// line its buildings stand on.
type cityRow struct {
	from, to int // half-open range into the ranked block list
	baseY    float64
}

// layoutCityRows splits n ranked repos into balanced rows no wider than budget.
// Balanced rather than greedy: 19 against a budget of 8 comes out 7+7+5, not
// 8+8+3, so no street is left holding a stub of three buildings and a lot of
// empty asphalt.
func layoutCityRows(n, budget int) (rows, cols int) {
	if n <= budget {
		return 1, n
	}
	rows = int(math.Ceil(float64(n) / float64(budget)))
	cols = int(math.Ceil(float64(n) / float64(rows)))
	return rows, cols
}

func renderCity(s *Stats) string {
	blocks := s.CityBlocks()
	if len(blocks) == 0 {
		return renderEmpty("~/city", "No repositories with commit history to build from yet.")
	}

	const (
		pad      = 20.0
		skyTop   = 42.0  // first street's airspace begins here, under the chrome
		maxTower = 120.0 // tallest building, in px
		minTower = 14.0  // a one-commit repo is still a visible structure
		depth    = 16.0  // axonometric offset to the back-right
		minWide  = 46.0
		maxWide  = 76.0
		groundH  = 9.0
		pitch    = 105.0
		roofRoom = 24.0 // clearance over the tallest roof for its star badge
		labelH   = 36.0 // the two label lines hanging under a street
		rowGap   = 16.0
		nameSize = 9.5
		metaSize = 9.0
	)

	nRows, nCols := layoutCityRows(len(blocks), maxCityCols)

	peak := float64(blocks[0].Commits)
	maxSize := 0
	for _, r := range blocks {
		if r.SizeKB > maxSize {
			maxSize = r.SizeKB
		}
	}

	// One px-per-commit scale and one px-per-kilobyte scale across every row.
	// Scaling each row to its own leader would make a three-commit repo on the last
	// street as tall as the leader on the first, which is the one thing a ranked
	// chart may never do.
	towers := make([]float64, len(blocks))
	widths := make([]float64, len(blocks))
	for i, r := range blocks {
		towers[i] = minTower + (float64(r.Commits)/peak)*(maxTower-minTower)
		widths[i] = minWide
		if maxSize > 0 {
			// sqrt so one oversized repo does not squash every other footprint.
			widths[i] = minWide + math.Sqrt(float64(r.SizeKB)/float64(maxSize))*(maxWide-minWide)
		}
	}

	// Wrapping the street is what lets every repo stand. One row of nineteen would
	// be a 2035px card, which a README renders at 860px — 9.5px labels arriving as
	// 4px. Three rows of seven is 775px, under the column width, so the type lands
	// at full size and nothing has to be cut.
	//
	// Each street is then given only the airspace its own tallest building needs.
	// The scale stays global, so rows remain comparable; what varies is dead sky,
	// and reclaiming it is the difference between a card that fits and one that
	// scrolls.
	rows := make([]cityRow, 0, nRows)
	y := skyTop
	for from := 0; from < len(blocks); from += nCols {
		to := from + nCols
		if to > len(blocks) {
			to = len(blocks)
		}
		tall := 0.0
		for i := from; i < to; i++ {
			if towers[i] > tall {
				tall = towers[i]
			}
		}
		// Streets land on whole pixels: a 430.3657px card is a hairline rendered
		// twice at half opacity on every row it touches.
		rows = append(rows, cityRow{from: from, to: to, baseY: math.Round(y + roofRoom + tall)})
		y = rows[len(rows)-1].baseY + groundH + labelH + rowGap
	}
	legendY := rows[len(rows)-1].baseY + groundH + labelH + 16
	h := math.Ceil(legendY + 46)

	foot := "height = commits on the default branch  ·  footprint = repo size  ·  star = stargazers  ·  one scale across every row"
	w := 2*pad + float64(nCols)*pitch
	if need := math.Ceil(2*pad + textWidth(foot, 9)); need > w {
		w = need
	}

	live, dormant := 0, 0
	for _, r := range blocks {
		if r.PushedWithin(s.GeneratedAt, cityFreshWindow) {
			live++
		} else {
			dormant++
		}
	}

	// The description is the whole card for a screen reader, so the private count
	// belongs in it: it is the difference between a name a visitor can follow and
	// one they cannot. Reading order belongs in it too, because rank running across
	// then down is the one thing a sighted reader gets for free.
	order := ""
	if len(rows) > 1 {
		order = fmt.Sprintf(" Drawn as %d streets of up to %d, ranked left to right then top to bottom.",
			len(rows), nCols)
	}
	priv := ""
	if n := privateIn(blocks); n > 0 {
		verb := "are"
		if n == 1 {
			verb = "is"
		}
		priv = fmt.Sprintf(" %d %s private and shown for activity only.", n, verb)
	}
	if total := len(s.RankedRepos()); total > len(blocks) {
		priv += fmt.Sprintf(" %d further repositories are listed in the table view rather than drawn.",
			total-len(blocks))
	}
	c := newCanvas(w, h, "Repository skyline",
		fmt.Sprintf("%d repositories as buildings, ranked by commits on the default branch. "+
			"Building height is commit count, footprint is repo size, and a lit facade means pushed in the last 30 days. "+
			"%d occupied, %d dormant. Tallest is %s with %s commits.%s%s",
			len(blocks), live, dormant, blocks[0].Name, commas(blocks[0].Commits), order, priv))

	// Motion has to fail safe. A transform that scales a building up from zero
	// renders the whole card empty in any renderer that samples the first frame of
	// a running animation without advancing it — the same trap that blanked the
	// terminal card. So the geometry never animates: only the window fill does,
	// its base state is fully lit, and the worst a frozen frame can look is a city
	// at dusk. The wave comes from varying duration rather than delay, because a
	// delay would need a backwards fill and that is what hides content.
	//
	// The first keyframe is .3 rather than 0 for the same reason: a frozen frame
	// then shows dim windows instead of blank walls, so no stop in this file ever
	// renders a mark away. The step is 60ms so the ripple crosses a full three-row
	// city inside two seconds instead of trailing past it.
	c.style(".win{animation:lamps 900ms ease-out}")
	c.style("@keyframes lamps{from{opacity:.3}to{opacity:1}}")
	for i := range blocks {
		c.style(fmt.Sprintf(".w%d{animation-duration:%dms}", i, 620+i*60))
	}

	// Never let the title imply completeness the picture does not have: if the cap
	// cut anything, say how many of how many are standing.
	title := fmt.Sprintf("~/city  ·  %s ranked by commits", plural(len(blocks), "repo"))
	if total := len(s.RankedRepos()); total > len(blocks) {
		title = fmt.Sprintf("~/city  ·  top %d of %d repos by commits", len(blocks), total)
	}
	c.windowChrome(title)

	for _, row := range rows {
		// The street each row stands on: one recessive plinth, drawn before its
		// buildings so their bases sit on it rather than float.
		streetEnd := pad + float64(row.to-row.from-1)*pitch + widths[row.to-1] + depth
		c.poly(surfaceUp, []pt{
			{pad, row.baseY}, {pad + depth, row.baseY - depth/2},
			{streetEnd, row.baseY - depth/2}, {streetEnd, row.baseY},
		})
		c.rect(pad, row.baseY, streetEnd-pad, groundH, surfaceUp, 0)
		c.hRule(pad, streetEnd, row.baseY, baseline)

		for i := row.from; i < row.to; i++ {
			r := blocks[i]
			bx := pad + float64(i-row.from)*pitch
			th, bw := towers[i], widths[i]

			f := facadeDark
			state := "dormant"
			if r.PushedWithin(s.GeneratedAt, cityFreshWindow) {
				f = facadeLive
				state = "occupied"
			}

			access := ""
			if r.Private {
				access = " · private"
			}
			tip := fmt.Sprintf("#%d  %s — %s on the default branch · %s · %s · pushed %s (%s)%s",
				i+1, r.Name, plural(r.Commits, "commit"), humanBytes(r.SizeKB*1024),
				plural(r.Stars, "star"), r.PushedAt.Format("2 Jan 2006"), state, access)

			c.group(tip)
			drawBuilding(c, bx, row.baseY, bw, th, depth, f, fmt.Sprintf(`class="win w%d"`, i))
			c.groupEnd()

			// A landmark badge, not a size channel: the number is written out beside
			// a drawn star. The star is a path because ★ is outside monoStack's
			// coverage and this SVG is resolved against fonts on the reader's machine.
			if r.Stars > 0 {
				const starR, starGap = 4.0, 3.0
				label := commas(r.Stars)
				badgeW := starR*2 + starGap + textWidth(label, 9.5)
				left := bx + bw/2 - badgeW/2
				cy := row.baseY - th - depth/2 - 8
				c.star(left+starR, cy, starR, accent)
				// +3.4 puts the digits' cap height on the star's centre line.
				c.text(left+starR*2+starGap, cy+3.4, label,
					textOpts{size: 9.5, fill: inkHi, weight: "500", tooltip: tip})
			}

			// Every building is named and valued in text, so neither identity nor
			// magnitude depends on judging a 3D volume.
			c.text(bx, row.baseY+groundH+17, truncateToWidth(r.Name, nameSize, pitch-10),
				textOpts{size: nameSize, fill: f.label, weight: "500", tooltip: tip})
			meta := truncateToWidth(fmt.Sprintf("%s %s%s", f.glyph, commas(r.Commits), access),
				metaSize, pitch-10)
			c.text(bx, row.baseY+groundH+30, meta, textOpts{size: metaSize, fill: inkLow, tooltip: tip})
		}
	}

	renderCityLegend(c, pad, legendY, live, dormant)
	caption(c, pad, h-14, w-2*pad, foot)
	return c.String()
}

// drawBuilding lays down one axonometric box plus its window texture. Faces are
// drawn back-to-front so the near wall owns the silhouette's crisp edge.
func drawBuilding(c *canvas, bx, baseY, bw, th, depth float64, f facade, winAttrs string) {
	dy := depth / 2
	topY := baseY - th

	c.poly(f.side, []pt{
		{bx + bw, baseY}, {bx + bw, topY},
		{bx + bw + depth, topY - dy}, {bx + bw + depth, baseY - dy},
	})
	c.poly(f.top, []pt{
		{bx, topY}, {bx + bw, topY},
		{bx + bw + depth, topY - dy}, {bx + depth, topY - dy},
	})
	c.poly(f.front, []pt{
		{bx, baseY}, {bx + bw, baseY}, {bx + bw, topY}, {bx, topY},
	})

	drawWindows(c, bx, topY, baseY, bw, f.window, winAttrs)
}

// drawWindows fills the front wall with a uniform grid. The grid is texture; the
// one bit of data it carries is whether the wall is lit, which is why every
// window on a building shares a single state instead of varying per window — a
// varying pattern would look like a per-window value that does not exist.
func drawWindows(c *canvas, bx, topY, baseY, bw float64, fill, attrs string) {
	const (
		winW    = 4.0
		winH    = 5.0
		colStep = 10.0
		rowStep = 13.0
		inset   = 8.0
	)
	cols := int(math.Floor((bw - 2*inset + (colStep - winW)) / colStep))
	if cols < 1 {
		return
	}
	// Centre the grid on the wall rather than letting it drift left.
	gridW := float64(cols)*colStep - (colStep - winW)
	x0 := bx + (bw-gridW)/2

	// One group per wall, so the whole lamp-up is a single animated element
	// instead of one animation per window.
	c.raw(`    <g %s>`, attrs)
	for y := topY + 9; y+winH <= baseY-7; y += rowStep {
		for i := 0; i < cols; i++ {
			c.rect(x0+float64(i)*colStep, y, winW, winH, fill, 1)
		}
	}
	c.raw(`    </g>`)
}

// renderCityLegend names the two facade states. Two categories means a legend is
// required, and it is paired with the same glyph each building carries so the
// mapping holds even where the swatch colour is lost.
func renderCityLegend(c *canvas, x, y float64, live, dormant int) {
	items := []struct {
		f     facade
		label string
	}{
		{facadeLive, fmt.Sprintf("%s occupied — pushed in the last 30 days", plural(live, "repo"))},
		{facadeDark, fmt.Sprintf("%s dormant", plural(dormant, "repo"))},
	}
	for _, it := range items {
		c.rect(x, y-7, 9, 9, it.f.top, 2)
		c.rect(x+2, y-5, 5, 5, it.f.window, 1)
		c.text(x+15, y, fmt.Sprintf("%s  %s", it.f.glyph, it.label),
			textOpts{size: 9.5, fill: inkMid})
		x += 22 + textWidth(it.f.glyph+"  "+it.label, 9.5) + 26
	}
}

// cityAge is the freshness cut the card renders, exposed so the README table can
// state the same window instead of restating a magic number.
func cityAge() string { return plural(int(cityFreshWindow/(24*time.Hour)), "day") }

// privateIn counts the private repos among the drawn buildings, which is not the
// same as the profile's total: the skyline is capped.
func privateIn(blocks []Repo) int {
	n := 0
	for _, r := range blocks {
		if r.Private {
			n++
		}
	}
	return n
}
