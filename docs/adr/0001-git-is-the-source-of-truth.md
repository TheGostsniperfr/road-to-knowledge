# ADR-0001: The knowledge repository is the source of truth

- Status: accepted, 2026-11-08

## Context
Knowledge must survive the app, stay private, be diffable and be editable by hand and by agents.

## Options
1. A database owned by the app, exported to git.
2. Plain files in a private git repository, the app keeps only caches.

## Decision
Option 2. The app is public (`road-to-knowledge`), the data is private (`knowledge`). The
search index is rebuilt in memory on every load; nothing else is stored.

## Confirmation
Deleting every cache and restarting `rtk serve` shows the same universe.
