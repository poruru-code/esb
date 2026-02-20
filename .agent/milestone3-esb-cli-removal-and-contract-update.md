# Milestone 3 Plan: Remove cli/ from esb and Update Contracts

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

Make `esb` repository stable without `cli/`.

## Progress

- [x] (2026-02-20) Remove `cli/`, `go.work.cli`, `go.work.cli.sum`.
- [x] (2026-02-20) Update CI workflows to no-CLI steady state.
- [x] (2026-02-20) Update docs/scripts pointing to in-repo CLI paths.
- [x] (2026-02-20) Keep boundary/layout contracts aligned with post-split ownership.

## Surprises & Discoveries

- Observation: layout guard had residual CLI template requirements and required simplification for no-CLI steady state.
  Evidence: `check_repo_layout.sh` previously depended on `CLI_ABSENT_MODE` branch for missing `cli/assets`.
- Observation: developer bootstrap still assumed local CLI build.
  Evidence: `.mise.toml` and `lefthook.yml` contained local CLI-only tasks/hooks.

## Decision Log

- Decision: remove CLI-specific jobs/paths instead of keeping dormant compatibility shims.
  Rationale: dead compatibility paths increase maintenance cost and confusion.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 3 completed.

- `cli/` tree and `go.work.cli*` removed from `esb`.
- quality-gates updated to no-CLI steady state (`go-lint-cli` removed, boundary/compile jobs simplified).
- docs now point to `https://github.com/poruru-code/esb-cli`.
- local developer flow updated (`mise setup` validates installed `esb` command; no in-repo CLI build hooks).

## Revision Notes

- 2026-02-20: Initial milestone detail created.
