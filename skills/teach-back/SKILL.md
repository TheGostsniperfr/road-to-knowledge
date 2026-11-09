---
name: teach-back
description: Quiz the user on Road to Knowledge subjects they may be forgetting, with open questions they answer in their own words, then propose card and evidence updates. Uses the `knowledge` MCP server (at_risk, node, fragments, due, propose). Use when the user asks for a "teach-back", "interroge-moi", "quiz", "récap de ce que j'oublie", or a weekly recap session.
---

# Teach-back session

Goal: in ten minutes, find out what the user still knows on the subjects most at risk, and
leave behind better cards for what slipped. Explaining is the exercise: the user talks, you
listen and probe.

Needs the MCP server `knowledge`. Writes go through `propose` only.

## 1. Pick, at most three subjects

1. `at_risk(limit: 6)` (add `scope` if the user named a theme). Prefer subjects with notions
   and a long `last_review_days`; skip any the user says they do not care about.
2. For each pick: `node(id)`, then `fragments(id, section)` for one or two notions. Read
   `due(scope: id)` to see the existing cards so you do not duplicate them.
3. Tell the user the three subjects and why (the `reason` field), in one line each, and start.

## 2. Ask, one question at a time

- Open questions, never yes/no or multiple choice: "Explique-moi comment … ", "Que se passe-t-il
  si … ", "Quelle différence entre … et … ?". Start broad, then one follow-up on what was vague.
- Base every question on the fiche. If the fiche is thin, ask about the summary and say so.
- Do not give the answer before the user tries. After the answer: say what was right, what was
  missing, and the correct version in two or three sentences from the fiche.
- Grade each answer silently: solid, partial, forgotten. Two to four questions per subject.
- The user can say "passe" or "stop" at any time.

## 3. Propose what the session showed

One `propose` call, slug `teach-back-<YYYY-MM-DD>`, message `feat(review): teach-back du <date>`,
with the full current file of each subject (`fragments(id, raw: true)`) and:
- for each partial or forgotten point: a `question :: answer` card, short, in the user's
  language, unless an equivalent card already exists;
- for a subject answered solidly with nothing to add: no change.

Never change `level`. A forgotten subject keeps its level (ADR-0003): the new cards and the
review schedule bring it back. If the user explained something beyond the fiche, offer
to raise the level by one, and only change it if they say yes.

## 4. Close

Three lines: what is solid, what slipped, the branch to review
(`git diff main...proposals/teach-back-<date>`). Suggest a review session in the app for the new cards.
