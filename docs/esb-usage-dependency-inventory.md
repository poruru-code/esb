<!--
Where: docs/esb-usage-dependency-inventory.md
What: Inventory of ESB usages and dependency boundaries between esb and esb-branding-tool.
Why: Clarify what branding tool covers and what remains outside of coverage before implementation changes.
-->
# ESB使用箇所・依存関係インベントリ（Baseline: 2026-02-12）

## 0. Phase2/3 更新サマリー（2026-02-12）
この文書の 1 章以降は Phase2 実装前 baseline です。現状との差分は以下です。

- 実行系キーは中立名へ一本化済み:
  - `TAG`, `REGISTRY`, `SKIP_GATEWAY_ALIGN`, `REGISTRY_WAIT`, `CLI_BIN`
  - `META_REUSE`, `BUILDKITD_OVERWRITE`, `BUILDX_NETWORK_MODE`, `TINYPROXY_*`
- `runtime-safe` スコープ（docs/test除外）では `ESB_*` は 0 件。
- `e2e/environments/*.env` の `ESB_*` は中立キーへ更新済み（`ENV`, `TEMPLATE`, `PORT_*`）。
- `ESB_*` 残存は baseline 記述を含む docs のみ。

現状確認コマンド:

```bash
python3 - <<'PY'
import subprocess
import pathlib
import re

pat = re.compile(r"\bESB_[A-Z0-9_]+\b")
files = subprocess.check_output(["git", "ls-files"], text=True).splitlines()
hits = []
for rel in files:
    if rel.startswith("docs/") or rel.endswith(".md"):
        continue
    if rel.endswith("_test.go") or "/tests/" in rel or rel.startswith("e2e/scenarios/"):
        continue
    p = pathlib.Path(rel)
    try:
        text = p.read_text(encoding="utf-8")
    except Exception:
        continue
    c = len(pat.findall(text))
    if c:
        hits.append((rel, c))
print("runtime_scope_files_with_ESB_*", len(hits))
print("runtime_scope_total_ESB_*", sum(c for _, c in hits))
PY
```

## 1. 目的と判定基準
このドキュメントは、`/home/akira/esb` と `~/esb-branding-tool` における ESB 依存の現状を、実装修正前に固定化するための棚卸しです。

判定基準は以下に固定します。

- ESB 使用箇所の検出パターン（正規表現）:
  - `\bESB\b|\bESB_[A-Z0-9_]+\b|\besb\b|\.esb-info|esb_`
- `カバー済み`:
  - `~/esb-branding-tool/tools/branding/generate.py` の `TEMPLATES` に含まれる生成対象のみ。
- `未カバー`:
  - 上記以外（ヘッダー生成物だが非管理、通常ソース、テスト、ドキュメント、生成コードを含む）。
- 集計対象:
  - `git ls-files` で取得した追跡ファイルのみ。
- 除外対象:
  - `.esb/` と `e2e/.parallel-*.log`（作業生成物/ログ扱いのため）。

## 2. 調査方法（再現可能コマンド）
### 2.1 esb 側集計
```bash
git -C /home/akira/esb ls-files \
  | rg -v '^\.esb/|^e2e/\.parallel-.*\.log$' \
  > /tmp/esb-files.txt

python3 - <<'PY'
from pathlib import Path
import re

root = Path("/home/akira/esb")
pat = re.compile(r"\bESB\b|\bESB_[A-Z0-9_]+\b|\besb\b|\.esb-info|esb_")
files = Path("/tmp/esb-files.txt").read_text(encoding="utf-8").splitlines()

total = 0
hit_files = 0
for rel in files:
    p = root / rel
    try:
        text = p.read_text(encoding="utf-8")
    except Exception:
        continue
    c = len(pat.findall(text))
    if c:
        hit_files += 1
        total += c
print("files_scanned", len(files))
print("files_with_matches", hit_files)
print("total_matches", total)
PY
```

### 2.2 branding-tool 側集計
```bash
git -C /home/akira/esb-branding-tool ls-files > /tmp/tool-files.txt

python3 - <<'PY'
from pathlib import Path
import re

root = Path("/home/akira/esb-branding-tool")
pat = re.compile(r"\bESB\b|\bESB_[A-Z0-9_]+\b|\besb\b|\.esb-info|esb_")
files = Path("/tmp/tool-files.txt").read_text(encoding="utf-8").splitlines()

total = 0
hit_files = 0
for rel in files:
    p = root / rel
    try:
        text = p.read_text(encoding="utf-8")
    except Exception:
        continue
    c = len(pat.findall(text))
    if c:
        hit_files += 1
        total += c
print("files_scanned", len(files))
print("files_with_matches", hit_files)
print("total_matches", total)
PY
```

### 2.3 カバー判定（`generate.py` の `TEMPLATES` 基準）
```bash
python3 - <<'PY'
from pathlib import Path
import re

tool = Path("/home/akira/esb-branding-tool")
text = (tool / "tools/branding/generate.py").read_text(encoding="utf-8")
spec = re.compile(r'TemplateSpec\(\s*"([^"]+)"\s*,\s*"([^"]+)"\s*,?\s*\)', re.S)

for tpl, target in spec.findall(text):
    print(target, "<=", tpl)
PY
```

## 3. 現状サマリー（2026-02-12）
### 3.1 全体数値
| Repo | Scanned tracked files | Files with ESB pattern | Total matches |
|---|---:|---:|---:|
| `/home/akira/esb` | 632 | 155 | 810 |
| `/home/akira/esb-branding-tool` | 18 | 11 | 231 |

### 3.2 esb 側のカバー内/外
| 区分 | Files with matches | Total matches |
|---|---:|---:|
| `カバー済み`（`generate.py:TEMPLATES` に含まれる9ターゲット） | 9 | 194 |
| `未カバー`（上記以外） | 146 | 616 |

### 3.3 `ESB_*` 環境変数トークン（esb 側、出現ファイル数）
主なもの:

- `ESB_ENV`: 7
- `ESB_REGISTRY`: 5
- `ESB_TAG`: 5
- `ESB_CLI`: 3
- `ESB_SKIP_GATEWAY_ALIGN`: 3
- `ESB_META_REUSE`: 2
- `ESB_OUTPUT_DIR`: 2
- `ESB_REGISTRY_WAIT`: 2
- `ESB_TINYPROXY_*`: 複数（`tools/e2e_proxy/run_with_tinyproxy.py` ほか）

参照例: `docker-compose.docker.yml`, `e2e/runner/env.py`, `cli/internal/usecase/deploy/gateway_runtime.go`, `tools/dind-bundler/build.sh`, `tools/buildkit/setup_buildx.py`。

## 4. カバー範囲（branding toolでできている箇所）
`~/esb-branding-tool/tools/branding/generate.py` の `TEMPLATES` で管理される9ターゲットは以下です。

| Target | Template | Template内 ESB パターン数 | 備考 |
|---|---|---:|---|
| `config/defaults.env` | `tools/branding/templates/config/defaults.env.tmpl` | 0 | `CLI_CMD`/`ENV_PREFIX` はプレースホルダ化済み |
| `Makefile` | `tools/branding/templates/Makefile.tmpl` | 2 | `ESB_REGISTRY` が残存 |
| `meta/meta.go` | `tools/branding/templates/meta/meta.go.tmpl` | 0 | ブランド派生値で生成 |
| `docker-compose.docker.yml` | `tools/branding/templates/docker-compose.docker.yml.tmpl` | 8 | `ESB_REGISTRY`, `ESB_TAG` 等が残存 |
| `docker-compose.containerd.yml` | `tools/branding/templates/docker-compose.containerd.yml.tmpl` | 14 | 同上 + `esb_cni_*` が残存 |
| `docker-compose.fc.yml` | `tools/branding/templates/docker-compose.fc.yml.tmpl` | 14 | 同上 |
| `docker-compose.fc-node.yml` | `tools/branding/templates/docker-compose.fc-node.yml.tmpl` | 14 | 同上 |
| `docker-compose.infra.yml` | `tools/branding/templates/docker-compose.infra.yml.tmpl` | 0 | ESB 固定文字列なし |
| `docker-bake.hcl` | `tools/branding/templates/docker-bake.hcl.tmpl` | 0 | `{{SLUG}}` ベースで生成 |

重要点:

- 「管理対象であること」と「ESB 固定が消えていること」は別です。
- 現状は管理対象内にも `ESB_TAG` / `ESB_REGISTRY` / `esb_cni_data` / `esb_cni_conf` が残っています（例: `docker-compose.containerd.yml`, `docker-compose.fc.yml`）。

## 5. 非カバー範囲（branding toolでできていない箇所）
### 5.1 ヘッダー上は生成物だが `generate.py` 非管理の5ファイル
| File | Header source | Template exists in `esb-branding-tool` | 状態 |
|---|---|---|---|
| `config/container-structure-test/agent.yaml` | `tools/branding/templates/config/container-structure-test/agent.yaml.tmpl` | No | 非管理 + 参照テンプレート欠落 |
| `config/container-structure-test/os-base.yaml` | `tools/branding/templates/config/container-structure-test/os-base.yaml.tmpl` | No | 非管理 + 参照テンプレート欠落 |
| `config/container-structure-test/python-base.yaml` | `tools/branding/templates/config/container-structure-test/python-base.yaml.tmpl` | No | 非管理 + 参照テンプレート欠落 |
| `entrypoint.sh` | `tools/branding/templates/entrypoint.sh.tmpl` | No | 非管理 + 参照テンプレート欠落 |
| `services/runtime-node/entrypoint.common.sh` | `tools/branding/templates/services/runtime-node/entrypoint.common.sh.tmpl` | No | 非管理 + 参照テンプレート欠落 |

### 5.2 通常ソース側（分類）
`未カバー` 616件を、次の4区分で扱います。

#### 1) 実行依存
実行時挙動に直接影響する箇所。

- `cli/internal/usecase/deploy/gateway_runtime.go`（`ESB_SKIP_GATEWAY_ALIGN`）
- `tools/dind-bundler/build.sh`（`ESB_ENV`, `ESB_OUTPUT_DIR`）
- `tools/buildkit/setup_buildx.py`（`ESB_BUILDKITD_OVERWRITE`, `ESB_BUILDX_NETWORK_MODE`）
- `services/agent/internal/runtime/containerd/ensure.go`（`esb-` コンテナID）

#### 2) 互換/運用依存（`ESB_*` 環境変数）
運用スクリプトやE2E設定で ESB 名を受ける箇所。

- `e2e/runner/constants.py`（`ENV_ESB_CLI`）
- `e2e/runner/utils.py`（`ESB_CLI`, `ESB_BIN`）
- `tools/e2e_proxy/run_with_tinyproxy.py`（`ESB_TINYPROXY_*`）
- `e2e/environments/e2e-firecracker/.env`（`ESB_PORT_*`, `ESB_TEMPLATE`）

#### 3) CI連携依存（dispatch/lock）
リポジトリ間連携で ESB 名を保持する箇所。

- `.github/workflows/branding-dispatch.yml`（`ESB_REPO`, `ESB_REF`）
- `~/esb-branding-tool/.github/workflows/branding-check.yml`（`esb_repo`, `esb_ref` input）
- `~/esb-branding-tool/tools/branding/update_lock.py`（`source.esb_repo`, `source.esb_commit`, `source.esb_ref`）
- `~/esb-branding-tool/branding.lock`

#### 4) テスト・文書・生成コード依存（非即時実行）
即時の実行機能より、検証・説明・生成由来の命名が主。

- テスト: `cli/internal/command/deploy_running_projects_test.go`, `e2e/runner/tests/test_context.py`
- 文書: `docs/container-runtime-operations.md`, `docs/branding-generator.md`
- 生成コード: `services/agent/pkg/api/v1/agent.pb.go`, `services/gateway/pb/agent_pb2_grpc.py`

## 6. 依存関係マップ
### チェーンA: `config/defaults.env` -> CLI/E2E/tools
`config/defaults.env` の `CLI_CMD` / `ENV_PREFIX` が、以下へ伝播しています。

- `cli/internal/infra/env/env_defaults_branding.go`（`CLI_CMD`, `ENV_PREFIX` を環境へ設定）
- `e2e/runner/utils.py`（`config/defaults.env` 読み込み）
- `tools/cert-gen/generate.py`（`CLI_CMD` 既定）
- `tools/dind-bundler/build.sh`（`CLI_CMD`, `ENV_PREFIX` 読み込み）

### チェーンB: `meta/meta.go` -> agent/runtime/cli
`meta/meta.go` のブランド値に広く依存。

- Agent: `services/agent/cmd/agent/main.go`, `services/agent/internal/cni/generator.go`
- Runtime naming: `services/agent/internal/runtime/docker/runtime.go`, `services/agent/internal/runtime/image_naming.go`
- CLI/paths: `cli/internal/infra/staging/staging.go`, `cli/internal/infra/build/go_builder_paths.go`

### チェーンC: `branding-dispatch` -> `branding-check` -> `branding.lock`
CI の連鎖依存。

- 起点: `/home/akira/esb/.github/workflows/branding-dispatch.yml`
- 受け側: `~/esb-branding-tool/.github/workflows/branding-check.yml`
- メタ更新: `~/esb-branding-tool/tools/branding/update_lock.py`
- 固定ファイル: `~/esb-branding-tool/branding.lock`

### チェーンD: `.esb-info` / `source.esb_*` の運用メタ依存（tool内）
ブランド生成運用での ESB 基準情報。

- `~/esb-branding-tool/tools/branding/generate.py`（`.esb-info`, `ESB_BASE_COMMIT`, `ESB_BASE_TAG`）
- `~/esb-branding-tool/tools/branding/update_lock.py`（`source.esb_*`）
- `~/esb-branding-tool/docs/branding-flow.md`（運用手順）

## 7. 結論（ギャップ明確化）
### 7.1 カバー済みだが ESB 固定が残る領域
`generate.py` 管理下でも、以下のテンプレートに ESB 固定が残存しています。

- `tools/branding/templates/Makefile.tmpl`
- `tools/branding/templates/docker-compose.docker.yml.tmpl`
- `tools/branding/templates/docker-compose.containerd.yml.tmpl`
- `tools/branding/templates/docker-compose.fc.yml.tmpl`
- `tools/branding/templates/docker-compose.fc-node.yml.tmpl`

### 7.2 そもそもカバー対象外の領域
`generate.py` 非管理の 146 ファイル（616件）に ESB 依存が残っています。
代表カテゴリは `cli/`（206件）、`services/`（195件）、`e2e/`（84件）、`tools/`（53件）。

### 7.3 現状ベースの最新計画（2026-02-12）
PR は 1 本化前提で進める。CI ガード追加は本計画スコープ外。

完了済み（esb リポジトリ側）:
1. 実行系の中立キー化（`TAG`, `REGISTRY`, `SKIP_GATEWAY_ALIGN`, `REGISTRY_WAIT`, `CLI_BIN`, `META_REUSE`, `BUILDKITD_OVERWRITE`, `BUILDX_NETWORK_MODE`, `TINYPROXY_*`）。
2. `runtime-safe` スコープで `ESB_*` が 0 件。
3. E2E 主要フレーク（非同期ログ到達待ち）を共通ヘルパー化して安定化。
4. E2E 環境ファイルの `ESB_*` を中立キーへ更新（`ENV`, `TEMPLATE`, `PORT_*`）。
5. テスト補助キー `ESB_FAKE_DOCKER_CALLS` を `FAKE_DOCKER_CALLS` に更新。

追加で完了済み（2026-02-12）:
1. `P0`: `esb-branding-tool` の未確定差分を確定し、`branding.lock` を現行 commit に更新（`66f056a`）。
2. `P0`: `esb-branding-tool` で `generate --check` を `brand=esb,acme,app` で実行し、`--force` なしで整合を確認。
3. `P0`: `esb` 側へ再生成を反映（差分ゼロ）し、`uv run python e2e/run_tests.py --parallel` の最終回帰を実施（PASS）。

次タスク（PR 1本化）:
1. `P1`: baseline 文書と現行運用文書を分離し、誤参照を防ぐ（本書は baseline、運用は `docs/branding-generator.md` を正とする）。
2. `P1`: PR本文に「Phase1/Phase2 実施済み」「`runtime-safe` スコープ `ESB_*` 0」「`--parallel` E2E PASS」を明記してレビュー投入する。

完了条件（DoD）:
1. PR 内の全コミットで `runtime-safe` スコープ `ESB_*` 0 を維持する。
2. `esb-branding-tool` の `branding-check` 相当検証が成功する。
3. `esb` 側で unit + E2E（`--parallel`）が成功する。
4. ドキュメント上で「baseline」と「現行方針」が明確に区別される。

## 8. 検証シナリオ（このドキュメントの妥当性）
1. `カバー済み9件` が `~/esb-branding-tool/tools/branding/generate.py` の `TEMPLATES` と一致する。
2. `非カバー5件` がヘッダー参照とテンプレート存在確認で一致する。
3. 数値サマリー（`810`, `231`, `194`, `616`）が `git ls-files` ベースで再計算一致する。
4. 各主張に最低1つの具体ファイル参照を持つ。
5. 「カバー判定は `TEMPLATES` 基準」が冒頭定義と各章で一貫している。
