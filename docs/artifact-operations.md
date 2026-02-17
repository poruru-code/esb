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
1. Generate phase: parse templates, build functions, produce artifacts
2. Apply phase: validate and merge artifact outputs into `CONFIG_DIR`, then provision
3. Runtime phase: run compose services and execute tests/invocations

## CLI Flow
### Generate only
```bash
esb artifact generate \
  --template e2e/fixtures/template.core.yaml \
  --template e2e/fixtures/template.stateful.yaml \
  --template e2e/fixtures/template.image.yaml \
  --env dev \
  --mode docker \
  --no-save-defaults
```

### Apply only
```bash
esb artifact apply \
  --artifact .esb/artifacts/<project>/<env>/artifact.yml \
  --out /path/to/config-dir \
  --secret-env /path/to/secrets.env
```

### Composite flow
`esb deploy` is a composite command:
- run generate for all templates
- write strict `artifact.yml`
- run apply once

## Non-CLI Apply Flow
Use `tools/artifactctl` as the canonical apply implementation.

```bash
tools/artifactctl validate-id --artifact /path/to/artifact.yml
tools/artifactctl apply \
  --artifact /path/to/artifact.yml \
  --out /path/to/config-dir \
  --secret-env /path/to/secrets.env

docker compose --profile deploy run --rm --no-deps provisioner
```

Notes:
- Shell wrappers must not implement merge/apply business logic.
- `tools/artifact/merge_runtime_config.sh` is a thin wrapper to `tools/artifactctl merge`.

## E2E Driver Modes
`e2e/environments/test_matrix.yaml` supports:

- `deploy_driver: cli`
  - uses `esb deploy`
- `deploy_driver: artifact`
  - producer step: `esb artifact generate` when `artifact_generate: cli`
  - consumer step: `artifactctl apply` + provisioner

`artifact_generate` modes:
- `cli`: run producer via CLI before apply
- `none`: skip producer; requires pre-existing `artifact.yml`

## Failure Policy
- Missing `artifact.yml`, required runtime config files, invalid ID, missing required secrets: hard fail
- Unknown `deploy_driver` or `artifact_generate` mode: hard fail
- Apply phase must not silently fall back to template-based sync paths
