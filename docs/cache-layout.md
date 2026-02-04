# Cache Layout (Planned)

Status: Implemented (project-scoped staging cache)

## Overview
This document defines the target cache layout for ESB deploy staging data.
Global configuration remains in the user home. Deploy merge results and staging
artifacts become project-scoped and live next to the template.

## Goals
- Keep global, reusable assets in `~/.esb`.
- Store deploy merge results per project/template to avoid cross-project bleed.
- Make cleanup obvious and local to the template directory.
- Remove the current hash-based staging directory names.

## Non-goals
- Changing buildx cache locations.
- Changing TLS, WireGuard, or other global assets.

## Current Behavior (Summary)
- Global config lives at `~/.esb/config.yaml`.
- Staging cache lives under `~/.esb/.cache/staging/<project-hash>/<env>/...`.
- The hash is derived from compose project + env, and env also appears as a
  subdirectory, which makes the layout redundant and hard to inspect.

## Target Behavior (Spec)
### Global (unchanged)
- `~/.esb/config.yaml` keeps recent templates and default inputs.
- `~/.esb/certs`, `~/.esb/wireguard`, `~/.esb/buildkitd.toml` remain global.

### Project-scoped (new default)
Use the template directory as the cache root:

```
<template_dir>/.esb/
  staging/
    <compose_project>/
      <env>/
        config/
          functions.yml
          routing.yml
          resources.yml
        services/
        pyproject.toml
        .deploy.lock
```

Notes:
- `compose_project` is the docker compose project name (PROJECT_NAME).
- `env` is the deploy environment (e.g., dev, staging).
- All staging artifacts are placed under `<compose_project>/<env>` to avoid
  cross-env collisions without a hash.

## Path Resolution Rules
The staging root should be resolved in this order:
1. `${ENV_PREFIX}_STAGING_DIR` (e.g., `ESB_STAGING_DIR`): absolute path to the staging root.
2. `${ENV_PREFIX}_STAGING_HOME` (e.g., `ESB_STAGING_HOME`): root directory; staging path becomes
   `<STAGING_HOME>/staging`.
3. Default to `<template_dir>/.esb/staging`.

If the template directory is not writable, fall back to
`$XDG_CACHE_HOME/esb/staging` and warn.

## Cleanup
- Remove a single env:
  `rm -rf <template_dir>/.esb/staging/<compose_project>/<env>`
- Remove all envs for a project:
  `rm -rf <template_dir>/.esb/staging/<compose_project>`

Global config and certs remain untouched.

## Compatibility Notes
- Env inference that currently scans `~/.esb/.cache/staging` must be updated
  to scan the new project-scoped staging root.
- The hash-based path is removed; any existing global cache should be treated
  as legacy and ignored once the new layout is in effect.
