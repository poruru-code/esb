## Summary

Describe what changed and why.

## Boundary Impact (Required)

- [ ] This PR changes `pkg/*` code.
- [ ] If `pkg/*` changed, at least two upper modules consume the new/shared behavior.
- [ ] If `pkg/*` changed, the change does not add command execution (`os/exec`) or runtime env branching (`CONTAINER_REGISTRY`, `HOST_REGISTRY_ADDR`) in pure-core packages.
- [ ] If `pkg/artifactcore` exported API changed, `tools/ci/artifactcore_exports_allowlist.txt` was updated with rationale below.

### Rationale for Shared Placement

Explain why this belongs in `pkg/*` instead of `cli/*` or `tools/*`.

### API Allowlist Change Rationale

If `tools/ci/artifactcore_exports_allowlist.txt` changed, explain why each new export is necessary.

## Validation

- [ ] `./tools/ci/check_tooling_boundaries.sh`
- [ ] `go -C tools/artifactctl test ./...`
- [ ] `go -C cli test ./...`
- [ ] `GOWORK=off go -C pkg/artifactcore test ./...`
- [ ] Additional tests (describe)

## Notes for Reviewers

Call out risky areas and anything requiring focused review.
