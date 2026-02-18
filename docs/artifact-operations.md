<!--
Where: docs/artifact-operations.md
What: Operational guide for artifact-first deploy flows with and without CLI.
Why: Make generate/apply responsibilities and commands explicit for operators.
-->
# Artifact Operations Guide

## Scope
This document defines operational flows for artifact-first deployment.

- Producer responsibility: generate artifacts (`artifact.yml` + runtime-config outputs)
- Applier responsibility: apply generated artifacts to `CONFIG_DIR` and run provisioner
- Runtime responsibility: consume prepared runtime-config only

The contract details live in `docs/deploy-artifact-contract.md`.

## Phase Model
1. Generate phase: parse templates and render artifact outputs (`artifact.yml`, runtime-config, Dockerfiles)
2. Image build phase: optional build/push of function images from rendered artifacts
3. Apply phase: validate and merge artifact outputs into `CONFIG_DIR`, then provision
4. Runtime phase: run compose services and execute tests/invocations

## CLI Flow
### Generate only
Default is render-only (`--build-images` is false by default):

```bash
esb artifact generate \
  --template e2e/fixtures/template.e2e.yaml \
  --env dev \
  --mode docker \
  --no-save-defaults
```

Notes:
- `esb artifact generate` does not merge outputs into `.esb/staging/**`.
- Generate writes template outputs + `artifact.yml`; merge/apply is `artifact apply` (or `tools/artifactctl apply`) responsibility.

### Generate with image build
```bash
esb artifact generate \
  --template e2e/fixtures/template.e2e.yaml \
  --env dev \
  --mode docker \
  --build-images \
  --no-save-defaults
```

### Apply only
```bash
esb artifact apply \
  --artifact .esb/artifacts/<project>/<env>/artifact.yml \
  --out /path/to/config-dir \
  --secret-env /path/to/secrets.env \
  --strict
```

### Composite flow
`esb deploy` is a composite command:
- run generate for all templates (build-only internal phase, image build enabled by deploy semantics)
- write strict `artifact.yml`
- run apply once

## Non-CLI Apply Flow
Use `tools/artifactctl` as the canonical apply implementation.

```bash
tools/artifactctl validate-id --artifact /path/to/artifact.yml

tools/artifactctl prepare-images \
  --artifact /path/to/artifact.yml

tools/artifactctl apply \
  --artifact /path/to/artifact.yml \
  --out /path/to/config-dir \
  --secret-env /path/to/secrets.env \
  --strict

docker compose --profile deploy run --rm --no-deps provisioner
```

Notes:
- Shell wrappers must not implement merge/apply business logic.
- `tools/artifact/merge_runtime_config.sh` is a thin wrapper to `tools/artifactctl merge`.
- `tools/artifactctl prepare-images` uses `<artifact_root>/runtime-base/**` as the only base-image build context. It does not read repository-local `runtime-hooks/**`.

## E2E Contract (Current)
`e2e/environments/test_matrix.yaml` is artifact-only:
- `deploy_driver` must be `artifact`
- `artifact_generate` must be `none`
- test execution consumes committed fixtures under `e2e/artifacts/*`
- firecracker profile is currently disabled in matrix (docker/containerd are active gates)

Fixture refresh is a separate developer operation (outside E2E runtime):
- regenerate fixtures with `e2e/scripts/regenerate_artifacts.sh`
- this script uses `esb artifact generate` and commits raw output
- E2E runner may build/push local fixture images from `tools/e2e-lambda-fixtures/*` when `image_uri_overrides` points to local fixture repos

## Failure Policy
- Missing `artifact.yml`, required runtime config files, invalid ID, missing required secrets: hard fail
- Unknown `deploy_driver` or unsupported `artifact_generate` mode: hard fail
- Missing runtime-base context for required base-image build in `prepare-images`: hard fail
- Apply phase must not silently fall back to template-based sync paths
- In `--strict`, runtime digest verification fails if `<artifact_root>/runtime-base/runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py` is missing or unreadable
- Removed runtime digests: `java_agent_digest`, `java_wrapper_digest`, `template_digest`
