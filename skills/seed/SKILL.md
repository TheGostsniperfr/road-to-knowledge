---
name: seed
description: Bootstrap or extend a Road to Knowledge repository from what the user has already done (GitHub repos, local projects, notes, CV). Proposes galaxies, sub-themes and subjects with levels backed by evidence, on a proposals/seed branch for review. Use when the knowledge repo is empty or when the user asks to "seed", "remplir", "importer mes projets".
---

# Seed a knowledge universe

Goal: a first universe the user recognises and corrects in ten minutes, not a perfect one.

## 1. Collect, cheaply

- `gh repo list <owner> --limit 200 --json name,description,primaryLanguage,isPrivate,updatedAt,repositoryTopics`
- For the 15 most recently updated repos: README head (`gh api repos/<o>/<r>/readme`), language breakdown, top-level files (Nix, Helm, Terraform, Dockerfile…).
- Local projects the user names: read `README`, `CLAUDE.md`, `docs/` headings only.
- Notes the user points to (Notion page, CV): headings and bullet points only.
- If an MCP server `knowledge` is connected, call `profile()` first and only add what is missing.

Never read secrets, `.env` files or credentials. Keep raw file dumps out of the main context: summarise per repo in two lines.

## 2. Ask, at most five questions

Themes the evidence cannot show (languages spoken, sports, cooking, books read), and the
level of the two or three subjects you are least sure about. Offer defaults.

## 3. Shape

- 4 to 8 galaxies. 2 to 6 sub-themes each. 3 to 15 subjects per sub-theme.
- Galaxy shape: `barred` for the biggest technical theme, `spiral` for big ones, `ring`
  for projects, `lenticular` for a library of books, `elliptical` or `irregular` for small ones.
- Levels: 1 heard of, 2 understood, 3 practised in a lab, 4 used in production, 5 can
  teach it. Never above 3 without evidence. Put the evidence URL in `evidence:`.
- Add 2 or 3 fog subjects (level 0, state `explore`) per galaxy: the natural next step
  from what the user knows. Explain why in the summary line.
- Links (wormholes) between a project and the subjects it uses, and a book and its topic.
- A short fiche only when you know something specific: summary, one or two `## notions`,
  a few `question :: answer` cards. Otherwise just the summary line.

## 4. Write on a branch

With the MCP server: one `propose` call per galaxy, `slug: seed`, conventional message
`feat(seed): <galaxy>`. Without it, never switch branches in the user's checkout:
`rtk serve` may be running on it and would commit review logs onto the proposal. Use a
temporary worktree:

```sh
cd "$RTK_DATA"
wt=$(mktemp -d)/seed
git worktree add -b proposals/seed "$wt" HEAD
# write files under "$wt", then for each galaxy:
rtk validate --data "$wt"
git -C "$wt" add galaxies/<galaxy> && git -C "$wt" commit -m "feat(seed): <galaxy>"
git worktree remove "$wt"
```

Never commit to main.

## 5. Hand over

Print a table galaxy · subjects · levels guessed · fog suggestions, the command to see
the result (`rtk serve --data <repo>` then switch to the branch, or
`git diff main...proposals/seed --stat`), and the questions you could not settle.
