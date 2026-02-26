# DeployOps CLI

`tools/deployops` はホスト側 deploy/bundle の統合 Python CLI です。
DinD 用アセット（`Dockerfile` / `entrypoint.sh`）も `tools/deployops/assets/dind/` に統合されています。

## Commands

```bash
# Artifact apply (auto-discover artifact/env/compose)
uv run python tools/deployops/cli.py apply

# DinD bundle (auto-discover artifact dir/env)
uv run python tools/deployops/cli.py bundle-dind \
  --prepare-images

# Pre-build only (auto-discover artifact dir/env)
uv run python tools/deployops/cli.py prepare-images \
  --dry-run

# You can still override explicitly
uv run python tools/deployops/cli.py apply \
  --artifact e2e/artifacts/e2e-docker/artifact.yml \
  --env-file e2e/environments/e2e-docker/.env \
  --compose-file e2e/environments/e2e-docker/docker-compose.yml
```

Dry-run はグローバル `--dry-run` で有効化します。
