<!--
Where: docs/branding-tool-separation-handoff.md
What: Meeting notes + handoff for branding tool separation work (revised).
Why: Preserve decisions, constraints, and executable next steps.
-->
# ブランディングツール分離: 打ち合わせメモ兼引継ぎ（改訂版）

## ゴール
- ブランディング変更ツールを本体リポジトリから分離し、下流向けのカスタマイズ時だけ使えるようにする。
- ベースリポジトリに下流固有情報を**一切含めない**。
- テンプレートの更新漏れを防ぎ、本体側は最小限の連携だけを残す。

## 前提・制約
- ベースは `CLI_CMD=esb` を維持（`config/defaults.env` を基準にする）。
- 下流固有の値・秘密情報はベースに含めない。
- 下流リポジトリにはツールを残さない（生成物のみ固定）。
- ベース変更はパッチ取り込み前提（履歴共有が理想）。
- 後方互換は不要（利用者はいない）。

## 決定事項（確定）
1) テンプレートは**外部ツールリポジトリ**にのみ置く。  
2) 下流でのブランド変更はツール側で実施し、生成物を下流で固定する。  
3) ツールの CI が **本体リポジトリのコミットをスナップショットとして保持**する。  
   - submodule は使わず、本体リポジトリの SHA / tag をツール側で管理する。
4) 本体側 CI に repository_dispatch を入れるのは許容。
5) ベースはツールに依存しない（コード・設定から参照しない）。

## 役割分担
### 本体リポジトリ
- `config/defaults.env` は常に `CLI_CMD=esb` / `ENV_PREFIX=ESB` を維持。
- ツール repo への `repository_dispatch` を送るワークフローのみ保持。
- `tools/branding` と `mise` の generator タスクは、ツール運用確立後に削除。

### ツールリポジトリ
- generator + templates + CI を保持。
- 本体リポジトリの SHA / tag を `branding.lock` に記録・更新。
- 本体リポジトリの生成物と一致するかを CI で検証。

### 下流リポジトリ
- ツール repo を使って生成物を作成し、**生成物のみ**コミット。
- ツール自体は repo に残さない。

## ツールリポジトリの初期構成（最小案）
`esb-branding-tool` を正とし、`tools/branding` はツール repo 側で管理する。

```
<tool-repo>/
  tools/branding/
    generate.py
    branding.py
    templates/...
  branding.lock
  .github/workflows/...
```

## 実行手順（この資料だけで開始できる形）
### 0) 実行環境の前提
```bash
# ツール repo 側で uv が使えること
uv --version
```

### 1) ツール repo を用意
```bash
# ツール repo を取得
git clone https://github.com/poruru-code/esb-branding-tool
```

### 2) ツール repo から本体リポジトリを検証（手動）
```bash
git clone <esb-repo-url> /tmp/esb-check
cd <tool-repo>
# ベースの生成物が正になる（本体 repo 内の生成結果と一致すること）
uv run python tools/branding/generate.py --root /tmp/esb-check --check --brand esb
```
本体側でテンプレート更新が入った場合は、ツール repo 側で更新したテンプレートを使う前提とする。
必要ならツール repo で `generate.py` を実行してから `--check` を行う。

### 3) 下流でブランド変更（手動）
```bash
git clone <downstream-repo-url> /tmp/downstream
cd <tool-repo>
uv run python tools/branding/generate.py --root /tmp/downstream --brand acme
cd /tmp/downstream
git add .
git commit -m "Branding: acme"
```

## 連携方式（確定）
### 本体 -> ツール CI 連携（repository_dispatch）
payload 例:
```json
{
  "event_type": "branding-check",
  "client_payload": {
    "esb_repo": "https://github.com/poruru-code/edge-serverless-box.git",
    "esb_commit": "<sha>"
  }
}
```

```
Upstream CI
  └── repository_dispatch (commit SHA)
      └── ツール CI: upstream commit を checkout
          └── branding check 実行
              ├── OK: ツール repo が branding.lock を更新して push
              └── NG: 本体側を fail させる（trigger-workflow-and-wait で結果取得）
```
本体側の運用方針:
- PR は同期チェックとして組み込む（trigger-workflow-and-wait で結果取得）。
- main は非同期で dispatch のみ（失敗時は通知/ログ）。
- ツール側修正後の回復は、本体側の再実行で再 dispatch する。

## 受入基準（ツール側のチェック）
- ベース repo で生成した成果物が**正**であり、ツールで生成した成果物が一致すること。
- 実装: `uv run python tools/branding/generate.py --root <esb_repo> --check --brand esb`

## branding.lock の仕様（暫定案）
ツール repo で管理する本体リポジトリのスナップショット情報。再現性と追跡性を優先する。

```yaml
schema_version: 1
locked_at: "2026-01-18T10:00:00Z"

tool:
  commit: "<sha>"
  ref: "<optional-tag-or-branch>"

source:
  esb_repo: "https://github.com/poruru-code/edge-serverless-box.git"
  esb_commit: "<sha>"
  esb_ref: "<optional-tag-or-branch>"

parameters:
  brand: "esb"
```

更新ルール:
- 更新主体はツール CI（branding check 成功時の commit/push）。
- 人手で `branding.lock` を直接編集しない。
- 例外対応は `workflow_dispatch` など CI の手動トリガー経由で更新する。
- `esb_commit` は必須。`esb_ref` は tag/branch 指定時のみ記録する。

## 参照メモ
- 下流運用フロー: https://github.com/poruru-code/esb-branding-tool/blob/main/docs/branding-flow.md

## 未決事項（次セッションで決める）
- ツール repo の versioning 方針（タグ / リリース / 本体 commit 追従の運用）
- `branding.lock` 仕様/更新ルールの最終確定
- CI 認証方式（PAT / GitHub App 等）

---

## Implementation references
- `config/defaults.env`

## 次のアクション案（新セッション）
1) ツール repo スケルトン作成と CI たたき台追加
2) branding.lock 仕様の確定
3) 本体 -> ツール CI 連携の実装
4) 本体リポジトリから `tools/branding` と `mise` タスクを削除
