# DinD E2E Equivalent Pytest

このディレクトリは、DinD コンテナを実体として E2E `pytest` を再現実行するためのスクリプトを提供します。

## 目的

- artifact から DinD イメージを作成
- DinD コンテナを起動
- DinD 内部 compose を対象に `pytest` を実行
- `restart` 系テストで必要な `docker compose` 操作も DinD 側に向けて実行

## クイックスタート

```bash
./tools/dind-bundler/dind-e2e-equivalent/run_dind_e2e_equivalent_pytest.sh
```

既定動作:

- ホスト Docker をクリーンアップして開始（破壊的）
- `e2e/artifacts/e2e-docker` と `e2e/environments/e2e-docker/.env` を利用
- `--prepare-images` 付きで DinD イメージをビルド
- DinD 起動後に 55 テスト（docker 想定セット）を実行

## 主なオプション

```bash
./tools/dind-bundler/dind-e2e-equivalent/run_dind_e2e_equivalent_pytest.sh \
  --artifact-dir e2e/artifacts/e2e-docker \
  --env-file e2e/environments/e2e-docker/.env \
  --image-tag esb-e2e-dind-repro:latest \
  --container-name esb-e2e-dind-repro \
  --keep-container
```

追加の `pytest` 引数は `--` 以降に渡せます。

```bash
./tools/dind-bundler/dind-e2e-equivalent/run_dind_e2e_equivalent_pytest.sh -- -k s3 -vv
```

## 注意

- `--skip-clean` を付けない限り、開始時に `docker system prune -af --volumes` を実行します。
- `restart` 系テストのため、`docker` コマンドは一時プロキシで DinD 内部 Docker に転送します。
