# `tools/cli` 利用ガイド（`esb-ctl`）

このドキュメントは、`tools/cli` 実装の使い方を 1 ファイルでまとめたものです。

## この CLI でできること

`esb-ctl` は次を提供します。

- artifact 適用（`deploy`）
- provisioner の明示実行（`provision`）
- stack 起動から deploy までの一括実行（`stack deploy`）
- オーケストレータ向け内部コマンド（`internal ...`）

## セットアップ

推奨（依存導入・フック有効化・ctl 用意をまとめて実行）:

```bash
mise run setup
```

`esb-ctl` のインストール/更新のみ実行:

```bash
mise run build-ctl
```

インストール確認:

```bash
esb-ctl --help
```

## コマンド一覧

```bash
esb-ctl deploy --artifact <artifact.yml> [--no-cache]
esb-ctl provision --project <name> --compose-file <file> [--compose-file <file2>] [--env-file <.env>] [--project-dir <dir>] [--with-deps] [-v]
esb-ctl stack deploy [--artifact <artifact.yml>]

esb-ctl internal maven-shim ensure --base-image <image> [--host-registry <host>] [--no-cache] [--output json]
esb-ctl internal fixture-image ensure --artifact <artifact.yml> [--no-cache] [--output json]
esb-ctl internal capabilities [--output json]
```

## 標準フロー（推奨）

ローカルでの基本運用は `stack deploy` を使います。

```bash
esb-ctl stack deploy
```

`artifacts/**/artifact.yml` が複数ある場合は、対話で選択できます。

artifact を明示指定する場合:

```bash
esb-ctl stack deploy --artifact artifacts/<name>/artifact.yml
```

`stack deploy` の実行内容:

1. artifact パス解決
2. compose stack 起動（`docker compose up -d`）
3. registry readiness 待機
4. `esb-ctl deploy --artifact ...` 実行
5. `esb-ctl provision ...` 明示実行

## 分離フロー（手動制御）

段階ごとに実行したい場合:

```bash
esb-ctl deploy --artifact artifacts/<name>/artifact.yml
esb-ctl provision \
  --project <compose-project> \
  --compose-file docker-compose.yml \
  --env-file .env \
  --project-dir "$(pwd)"
```

`--compose-file` は複数回指定とカンマ区切り指定の両方に対応します。

## `internal` コマンド（自動化向け）

契約バージョン確認:

```bash
esb-ctl internal capabilities --output json
```

fixture image 準備（JSON 出力）:

```bash
esb-ctl internal fixture-image ensure --artifact artifacts/<name>/artifact.yml --output json
```

maven shim image 準備（JSON 出力）:

```bash
esb-ctl internal maven-shim ensure --base-image public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7 --output json
```

## `stack deploy` で使う主な環境変数

- `CTL_BIN`: ctl バイナリ解決の上書き
- `PROJECT_NAME` / `ENV`: artifact 側に値がない場合の補完元
- `JWT_SECRET_KEY`: 必須（32 文字以上）
- `PORT_REGISTRY`: 既定 `5010`（`0` 指定で動的ポート）
- `REGISTRY_WAIT_TIMEOUT`: 既定 `60` 秒
- `REGISTRY_CONTAINER_NAME`: 既定 `esb-infra-registry`

## トラブルシュート

- `ctl command not found`:
  - `mise run build-ctl` を実行し、`~/.local/bin` が `PATH` に入っているか確認してください。
- registry コンテナ競合:
  - エラーメッセージに表示される既存の共有 registry コンテナを停止/削除して再実行してください。
- artifact が見つからない:
  - `--artifact` で明示指定してください。
- 使い方ヒント:
  - `esb-ctl --help`
  - `esb-ctl deploy --help`
  - `esb-ctl provision --help`
  - `esb-ctl stack --help`
