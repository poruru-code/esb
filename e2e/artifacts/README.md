# E2E Artifact Fixtures

These fixtures must be committed as raw output from `esb artifact generate`.
Do not edit files under this directory manually.

Regenerate all fixtures with:

```bash
./e2e/scripts/regenerate_artifacts.sh
```

If `esb` is not on your PATH, set `ESB_CMD` explicitly.

```bash
ESB_CMD='go -C cli run ./cmd/esb' ./e2e/scripts/regenerate_artifacts.sh
```

Notes:
- The script writes both generated files and `artifact.yml` directly into `e2e/artifacts/*`.
- The script uses environment-specific tags (`e2e-docker-latest`, `e2e-containerd-latest`) to avoid cross-environment image collisions.
- The script fails if the generated manifest mode does not match the requested mode.
