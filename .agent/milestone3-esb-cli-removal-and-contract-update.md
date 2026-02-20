# Milestone 3 Plan: Remove cli/ from esb and Update Contracts

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

Make `esb` repository stable without `cli/`.

## Progress

- [ ] Remove `cli/`, `go.work.cli`, `go.work.cli.sum`.
- [ ] Update CI workflows to no-CLI steady state.
- [ ] Update docs/scripts pointing to in-repo CLI paths.
- [ ] Keep boundary/layout contracts aligned with post-split ownership.

## Surprises & Discoveries

To be filled during implementation.

## Decision Log

- Decision: remove CLI-specific jobs/paths instead of keeping dormant compatibility shims.
  Rationale: dead compatibility paths increase maintenance cost and confusion.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

To be filled after implementation.

## Revision Notes

- 2026-02-20: Initial milestone detail created.
