# ADR-0007: V1 is local and desktop only

- Status: accepted, 2026-11-09 (Brian)

## Decision
`rtk serve` binds 127.0.0.1 on Brian's PC, started from the dev shell. No NixOS service
module. Mobile and the Claude mobile connector wait for V2-01, which brings public HTTPS
and OAuth 2.1 first.
