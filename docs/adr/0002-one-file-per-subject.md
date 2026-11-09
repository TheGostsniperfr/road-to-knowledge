# ADR-0002: One Markdown file per subject, hierarchy from folders

- Status: accepted, 2026-11-08

## Context
Three levels matter for the map: theme, sub-theme, subject. Notions and cards belong to a subject.

## Options
1. A single YAML graph file.
2. One file per subject, folders for the hierarchy, frontmatter for metadata.

## Decision
Option 2. Folder names are ids. `## headings` are notions (planets), `question :: answer`
lines are cards, the first paragraph is the summary. Metadata sits in YAML frontmatter
and the app rewrites only the keys it changes.

## Consequences
Renaming a folder changes ids; links and review history must follow (V2-08 will automate it).
