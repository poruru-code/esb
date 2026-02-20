# Milestone 2 Plan: Harden esb-cli as Standalone Repository

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

Ensure extracted CLI builds independently in `esb-cli`.

## Progress

- [x] (2026-02-20) Update module path to `github.com/poruru-code/esb-cli`.
- [x] (2026-02-20) Rewrite internal imports from old module prefix.
- [x] (2026-02-20) Run `go test ./...` in `esb-cli`.

## Surprises & Discoveries

- Observation: test assumptions about repo root and monorepo fixture layout broke after extraction.
  Evidence: failures in `internal/command`, `internal/infra/deploy`, and `internal/infra/env` before fixes.
- Observation: fallback to `projectDir` for compose provisioner root and local `testdata` fixture contract resolved standalone breaks without core behavior changes.
  Evidence: `GOWORK=off go test ./...` became green after those adjustments.

## Decision Log

- Decision: keep shared-core dependency direction (`esb-cli` depends on `github.com/poruru-code/esb/pkg/*`).
  Rationale: avoids copy-fork of core packages and preserves boundary rules.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 2 completed in `esb-cli`.

- Branch: `feat/standalone-module-init`
- Commit: `23e1ad9` (`feat: make cli standalone in esb-cli repo`)
- Validation: `GOWORK=off go test ./...` passed.

## Revision Notes

- 2026-02-20: Initial milestone detail created.
