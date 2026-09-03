# Setting up the cards generator

Everything the profile cards need in order to render private repositories and
keep refreshing themselves. Written 3 September 2026.

Two manual steps: push the pending commit, then install a token. After that the
workflow maintains itself — it re-renders daily at 06:17 UTC and on every push
that touches `cmd/cards/**`.

**Status: Part 1 is done. Only Part 2 remains.** The live cards are rendering
from the new code and their footer still reads "Public activity only", which is
the token's absence talking.

---

## Part 1 — Push the pending commit  ✅ done

Completed 3 September 2026: `32e05d7` reached the remote, the Action ran, and it
committed `3061325 cards: refresh from live profile data`. The live skyline now
reads `~/city · 9 repos ranked by commits`.

Kept as reference, because the conflict in 1.2 recurs. `assets/*.svg` and the
README table block are generated files with two writers — this Action and any
local render — so every local `go run ./cmd/cards` without `-out` guarantees the
next pull conflicts. See 4.3 for the way around it.

**1.1** Go to the repo:

```console
cd /home/student/Daboggieman
```

**1.2** Confirm the state before touching anything:

```console
git status -sb
```

You want one line, with no files listed under it:

```
## master...origin/master [ahead 1]
```

- `[ahead 1, behind 1]`, or modified files listed — the Action has pushed since
  this was written. Run `git pull --rebase` and re-check. If that conflicts, the
  conflict will be in `assets/*.svg` or the README table block; those are
  generated files, so take the remote's version: `git checkout --ours <path>`
  during the rebase, then `git rebase --continue`.
- No mention of `ahead` — the push already happened. Skip to Part 2.

**1.3** Push:

```console
git push
```

Expected:

```
To https://github.com/Daboggieman/Daboggieman.git
   9a286be..32e05d7  master -> master
```

**1.4** That push touches `cmd/cards/**`, a trigger path in
[.github/workflows/cards.yml](.github/workflows/cards.yml), so the Action starts
within seconds. Watch it at
<https://github.com/Daboggieman/Daboggieman/actions> — about 40 seconds. It ends
by committing `cards: refresh from live profile data`: the assets and README
table re-rendered by the new code.

Still public-only until Part 2.

---

## Part 2 — CARDS_TOKEN

This is the only thing between the cards and private repositories. Without it
the workflow falls back to the built-in `GITHUB_TOKEN`, which cannot see private
repos at all, so every Access cell in the table reads `public`.

### 2a — Mint the token

**2.1** Go to <https://github.com/settings/tokens>.

**2.2** If the page shows "Fine-grained tokens", click **Tokens (classic)** in
the left sidebar. It has to be a classic token: the GraphQL
`contributionsCollection` query this generator uses is only reliably supported
by classic tokens.

**2.3** **Generate new token** dropdown (top right) → **Generate new token
(classic)**.

**2.4** Confirm password or 2FA if prompted.

**2.5** **Note**: `profile-cards`

**2.6** **Expiration**: 90 days, or **No expiration** to never think about it
again. Either is safe — an expired token makes the workflow fail loudly with
`graphql http 401` rather than quietly reverting to public-only. That check is
[cmd/cards/github.go](cmd/cards/github.go), the `graphql http %d` error.

**2.7** Under **Select scopes**, tick exactly two things:

- **`repo`** — the top-level checkbox, "Full control of private repositories".
  Ticking it auto-ticks its five children (`repo:status`, `repo_deployment`,
  `public_repo`, `repo:invite`, `security_events`). Leave those as they land.
- **`read:user`** — scroll to the `user` group and tick only this child, not the
  `user` parent.

Nothing else. No `delete_repo`, no `admin:*`, no `workflow`.

**2.8** Scroll to the bottom → green **Generate token**.

**2.9** The token is shown once, starting `ghp_`. Copy it. Keep the tab open
until 2b is done; if you lose it you have to regenerate.

### 2b — Install it as a repository secret

**2.10** Go to
<https://github.com/Daboggieman/Daboggieman/settings/secrets/actions>.

**2.11** Green **New repository secret**.

**2.12** **Name** — exactly this, case-sensitive. A typo here is the most common
failure:

```
CARDS_TOKEN
```

**2.13** **Secret** — paste the `ghp_...` value.

**2.14** **Add secret**. `CARDS_TOKEN` now appears under "Repository secrets"
with an "Updated now" timestamp. GitHub will never show you the value again;
that is expected.

**2.15** Close the token tab.

### 2c — Run it

Adding a secret triggers nothing, so run the workflow by hand.

**2.16** Go to
<https://github.com/Daboggieman/Daboggieman/actions/workflows/cards.yml>.

**2.17** **Run workflow** (right side) → leave the branch as **master** → green
**Run workflow**.

**2.18** Wait ~5 seconds, refresh, click the top run, click the **render** job,
expand the **Render cards** step.

---

## Part 3 — Confirm it worked

**3.1** The last line of the "Render cards" log is the proof:

```
Daboggieman: 1552 contributions, 12 day streak (best 25), 7 languages, 2 of 10 repos private
```

`N of M repos private` is the part that matters. N of 1 or more means the token
is working. `0 of N` means it is not — see Troubleshooting.

**3.2** Open <https://github.com/Daboggieman> and hard-refresh
(<kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>R</kbd>). GitHub's image proxy caches the
SVGs, so without the hard refresh you may see the old skyline for a few minutes.
Extra buildings should be standing, `Kairo-v1` among them.

**3.3** Expand "Open the table view" at the bottom of the profile. Private repos
appear as plain names with `private` in the Access column and no link —
deliberately: the link would 404 for every visitor. Public repos stay linked.

**3.4** Expect the chrome to change wording. There are 9 public repos and the
skyline's cap is 9, so it currently reads `9 repos ranked by commits`. Once
private repos push the total past 9 it becomes `top 9 of M repos by commits`,
and the ones that did not fit are in the table view rather than missing. That is
the cap doing its job, not a bug: past ~1020px of card a README column downscales
the image until the labels are unreadable.

**3.5** Pull the Action's commits so the local clone stops drifting:

```console
cd /home/student/Daboggieman
git pull
```

Nothing further is manual from here.

---

## Part 4 — Optional extras

**4.1 Make GitHub's own contribution graph agree with the card.** The card
counts private contributions; GitHub's green heatmap hides them unless you opt
in. Go to <https://github.com/settings/profile>, scroll to **Contributions**,
tick **Include private contributions on my profile**.

**4.2 Keep a repo off the page entirely.** The token publishes each private
repo's name, commit count, size on disk and last-push date — not its code, but
the name is public. For anything that should not be named at all, go to
<https://github.com/Daboggieman/Daboggieman/settings/variables/actions>, click
**New repository variable**, name it `CARDS_EXCLUDE`, value a comma-separated
list:

```
some-private-repo,another-one
```

Matching is case-insensitive and exclusion happens before anything is counted,
so an excluded repo leaves no trace in the totals, the skyline, or the table.

**4.3 Preview locally without dirtying the tree.** Local renders overwrite the
same generated files the Action commits, which guarantees a pull conflict. Render
to a scratch directory instead:

```console
go run ./cmd/cards -fixture testdata/profile.json -out /tmp/cards -readme ""
```

`-buildings N` overrides how many the skyline draws; the rest stay in the table.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `0 of N repos private` | Secret name is not exactly `CARDS_TOKEN`, or `repo` scope missing | Re-check the name; regenerate with `repo` ticked |
| `graphql http 401` | Token expired, revoked, or pasted truncated | Generate a new token, update the secret |
| `graphql http 403` | Secondary rate limit | Re-run the job in 10 minutes |
| No `cards` workflow under Actions | Actions disabled | Settings → Actions → General → Allow all actions |
| Fails at **Commit** on `git push` | Branch protection on `master` | Settings → Branches → let Actions bypass, or drop the rule |
| `cards unchanged` | Nothing differed from the last render | Success — no commit needed |
| Profile still looks old | Camo image cache | Hard-refresh; clears within minutes |
