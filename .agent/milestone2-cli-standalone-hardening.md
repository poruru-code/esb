# Milestone 2 Plan: Harden esb-cli as Standalone Repository

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

Ensure extracted CLI builds independently in `esb-cli`.

## Progress

- [ ] Update module path to `github.com/poruru-code/esb-cli`.
- [ ] Rewrite internal imports from old module prefix.
- [ ] Run `go test ./...` in `esb-cli`.

## Surprises & Discoveries

To be filled during implementation.

## Decision Log

- Decision: keep shared-core dependency direction (`esb-cli` depends on `github.com/poruru-code/esb/pkg/*`).
  Rationale: avoids copy-fork of core packages and preserves boundary rules.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

To be filled after implementation.

## Revision Notes

- 2026-02-20: Initial milestone detail created.
