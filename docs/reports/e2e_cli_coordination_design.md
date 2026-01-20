<!--
Where: docs/reports/e2e_cli_coordination_design.md
What: CLI feature design to reduce E2E runner coupling and waste.
Why: Provide implementable CLI changes that make E2E stable and hermetic.
-->
# E2E/CLI Coordination Design (Implementation-Focused)

対象: `e2e/run_tests.py`, `e2e/runner/*`, `cli/internal/*`  
目的: Runner の内部依存を削減し、CLI で安定した API を提供する

---

## Goals

1. Runner が `ports.json` やコンテナ env を直接触らずにテストデータを取得できること。
2. Runner が CLI のグローバル設定を汚さずに独立して実行できること。
3. クリーンアップを CLI に集約し、Runner 側の Docker 直叩きを削減すること。

## Non-Goals

- E2E テストケースの内容変更や test matrix の再設計。
- CLI のユーザー向け出力の大幅変更（JSON 出力は追加のみ）。

---

## Proposed CLI Additions

### A) JSON 出力の標準化

#### `esb info --format json`
- **用途**: Runner が CLI の現在状態を機械可読で取得。
- **内容**:
  - `version`, `project`, `env`, `state`, `paths` など。
  - `--include-ports` で `ports.json` の内容を含める（Docker を起動しない）。

#### `esb up --format json`
- **用途**: E2E が `ports`/`credentials` を 1 回の実行で取得。
- **内容**:
  - `ports` (discovered), `credentials` (生成された場合のみ) を含める。
  - セキュリティのため `--include-credentials` を明示フラグにしてもよい。

#### `esb ports --format json`
- **用途**: `ports.json` の読み出しを CLI の API に置き換え。
- **オプション**:
  - `--refresh` (Docker から再検出して更新)
  - `--env` (対象環境指定)
  - `--format env|json|table`

### B) 認証情報 API

#### `esb credentials show --format json`
- **用途**: Runner がコンテナ env ではなく CLI で認証情報を取得。
- **実装案**:
  - `EnsureAuthCredentials` 実行時に `credentials.json` を `ESB_HOME/<env>/` に保存。
  - ファイルは `0600` で作成し、`--mask` オプションで伏字表示可。

### C) 設定の完全隔離

#### `--config-home` / `--config-path` グローバルフラグ
- **用途**: Runner が `~/.esb/config.yaml` を汚染しない。
- **実装案**:
  - 既存の `ESB_CONFIG_HOME` / `ESB_CONFIG_PATH` をフラグで上書き可能にする。

#### `--home` (ESB_HOME) グローバルフラグ
- **用途**: `ports.json` / `credentials.json` を Runner 用の隔離領域に集約。
- **実装案**:
  - `constants.HostSuffixHome` に対応するフラグ追加。

### D) 環境同期コマンド

#### `esb project sync --env <envs>`
- **用途**: Runner の matrix と generator.yml のズレを解消。
- **実装案**:
  - 既存 `generator.yml` の環境リストだけを更新。
  - `--dry-run` で差分確認可能。

（代替）`esb project add --force-env`  
既存の `project add` に環境更新の挙動を追加する案。

### E) クリーンアップの集約

#### `esb cleanup --hard`
- **用途**: Runner の `docker rm` / `docker volume rm` を廃止。
- **実装案**:
  - `down --volumes` + `prune --volumes` + 追加のラベル対象削除。
  - `--yes` で非インタラクティブ対応。

### F) 冪等性の明示

#### `esb stop` / `esb down`
- **改善案**:
  - `--ignore-missing` (未起動でも成功扱い) を追加。
  - E2E ではデフォルトで冪等動作に寄せられるのが望ましい。

---

## Data/Output Schema (Draft)

### `esb info --format json`
```json
{
  "version": "x.y.z",
  "project": { "name": "demo", "dir": "...", "generator_path": "...", "template_path": "..." },
  "env": { "name": "local", "mode": "docker", "state": "running" },
  "paths": { "output_dir": "...", "config_path": "...", "home_dir": "..." }
}
```

### `esb ports --format json`
```json
{
  "env": "local",
  "ports": { "PORT_GATEWAY_HTTPS": 443, "PORT_AGENT_GRPC": 50051 }
}
```

### `esb credentials show --format json`
```json
{
  "auth_user": "...",
  "auth_pass": "...",
  "jwt_secret_key": "...",
  "x_api_key": "...",
  "rustfs_access_key": "...",
  "rustfs_secret_key": "..."
}
```

---

## Implementation Steps (Minimal Path)

1. `ports` の読み出し/更新 API を CLI に追加（`ports.json` の直接アクセスを排除）。
2. `credentials.json` の保存と `credentials show` を追加（Runner が container env を参照しない）。
3. `--config-home` / `--home` の追加で Runner を完全隔離。
4. `project sync` または `project add --force-env` を追加。
5. `cleanup --hard` の追加と Runner の Docker 直叩き削除。

---

## Acceptance Criteria

- Runner が `ports.json` を直接読むコードを削除できる。
- Runner が `esb env var gateway` を使わずに認証情報を取得できる。
- Runner 実行で `~/.esb/config.yaml` が汚染されない。
- Runner から `docker rm`/`docker volume rm` が不要になる。
