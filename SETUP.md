# Setting up the cards generator

Everything the profile cards need in order to render private repositories and
keep refreshing themselves. Written 3 September 2026.

There are exactly two manual steps — push the pending commit, then install a
token. After that the workflow maintains itself: it re-renders daily at 06:17
UTC and on every push that touches `cmd/cards/**`.

---

## Part 1 — Push the pending commit

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
