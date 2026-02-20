<!--
Where: docs/repo-layout-contract.md
What: Repository layout contract for service/runtime/producer separation.
Why: Keep artifact-first boundaries explicit and enforceable in CI.
-->
# Repository Layout Contract

## Goal
Artifact-first deploy and future repo split depend on clear ownership boundaries:
- Service runtime behavior stays in core services/runtime hooks.
- Producer-only generation assets stay outside this repository.
- Shared contracts stay in dedicated contract directories.

## Canonical Ownership
- Producer renderer assets: managed outside this repository
- Runtime-required hooks: `runtime-hooks/**`
- Cross-service proto contracts: `services/contracts/proto/**`
- Host bootstrap tooling: `tools/bootstrap/**`
- Gateway runtime config assets: `services/gateway/config/**`
- runtime-node runtime config assets: `services/runtime-node/config/**`
- Image validation assets: `tools/container-structure-test/**`

## Prohibited Legacy Paths
These paths are retired and must not be reintroduced:
- `runtime/java/templates/**`
- `runtime/python/templates/**`
- `runtime/java/extensions/**`
- `runtime/python/extensions/**`
- `runtime/python/docker/Dockerfile`
- `contracts/proto/**` (repo-root contracts directory)
- `proto/agent.proto`
- `bootstrap/**` (repo-root bootstrap directory)
- `config/**` (repo-root config directory)

## Acceptance Criteria
1. `docker compose up` works with canonical paths only.
2. `uv run e2e/run_tests.py --parallel --verbose` passes without compatibility fallbacks for retired paths.
3. CI guard `tools/ci/check_repo_layout.sh` passes.

---

## Implementation references
- `runtime-hooks/java/README.md`
- `services/contracts/README.md`
- `services/contracts/buf.gen.yaml`
- `tools/bootstrap/README.md`
- `services/gateway/config/gateway_log.yaml`
- `services/gateway/config/haproxy.gateway.cfg`
- `services/runtime-node/config/Corefile`
- `tools/container-structure-test/os-base.yaml`
- `tools/ci/check_repo_layout.sh`
