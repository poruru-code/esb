<!--
Where: docs/branding-tool-separation-handoff.md
What: Meeting notes + handoff for branding tool separation work (revised).
Why: Preserve decisions, constraints, and executable next steps.
-->
# ブランディングツール分離: 打ち合わせメモ兼引継ぎ（改訂版）

## ゴール
- ブランディング変更ツールを ESB 本体から分離し、下流向けのカスタマイズ時だけ使えるようにする。
- ベースリポジトリに下流固有情報を**一切含めない**。
- テンプレートの更新漏れを防ぎ、ESB 側は最小限の連携だけを残す。

## 前提・制約
- ベースは `brand: esb` を維持（`config/branding.yaml` は esb）。
- 下流固有の値・秘密情報はベースに含めない。
- 下流リポジトリにはツールを残さない（生成物のみ固定）。
- ベース変更はパッチ取り込み前提（履歴共有が理想）。
- 後方互換は不要（利用者はいない）。

## 決定事項（確定）
1) テンプレートは**外部ツールリポジトリ**にのみ置く。  
2) 下流でのブランド変更はツール側で実施し、生成物を下流で固定する。  
3) ツールの CI が **ESB のコミットをスナップショットとして保持**する。  
   - submodule は使わず、ESB の SHA / tag をツール側で管理する。
4) ESB 側 CI に repository_dispatch を入れるのは許容。
5) ベースはツールに依存しない（コード・設定から参照しない）。

## 役割分担
### ESB リポジトリ
- `config/branding.yaml` は常に `brand: esb` を維持。
- ツール repo への `repository_dispatch` を送るワークフローのみ保持。
- `tools/branding` と `mise` の generator タスクは、ツール運用確立後に削除。

### ツールリポジトリ
- generator + templates + CI を保持。
- ESB の SHA / tag を `branding.lock` に記録・更新。
- ESB の生成物と一致するかを CI で検証。

### 下流リポジトリ
- ツール repo を使って生成物を作成し、**生成物のみ**コミット。
- ツール自体は repo に残さない。

## ツールリポジトリの初期構成（最小案）
現状の `tools/branding` をそのまま移植する前提。

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
# 新規 repo 作成後、ESB から tools/branding を移植
git clone <tool-repo-url>
cp -R <esb-repo>/tools/branding tools/branding
git add tools/branding
git commit -m "Tool: import branding generator"
```

### 2) ツール repo から ESB を検証（手動）
```bash
git clone <esb-repo-url> /tmp/esb-check
cd <tool-repo>
# ベースの生成物が正になる（ESB repo 内の生成結果と一致すること）
uv run python tools/branding/generate.py --root /tmp/esb-check --check --brand esb
```
ESB 側でテンプレート更新が入った場合は、`/tmp/esb-check` 側で生成物が最新であることを前提とする。
必要なら ESB 側で `tools/branding/generate.py` を実行してから `--check` を行う。

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
### ESB -> ツール CI 連携（repository_dispatch）
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
ESB CI
  └── repository_dispatch (commit SHA)
      └── ツール CI: ESB commit を checkout
          └── branding check 実行
              ├── OK: ツール repo が branding.lock を更新して push
              └── NG: ESB 側を fail させる（trigger-workflow-and-wait で結果取得）
```
ESB 側は「PR チェック」または「main への merge チェック」に組み込み、常に tool 側の検証が走るようにする。

## 受入基準（ツール側のチェック）
- ベース repo で生成した成果物が**正**であり、ツールで生成した成果物が一致すること。
- 実装: `uv run python tools/branding/generate.py --root <esb_repo> --check --brand esb`

## branding.lock の仕様（暫定案）
ツール repo で管理する ESB スナップショット情報。

```yaml
esb_repo: https://github.com/poruru-code/edge-serverless-box.git
esb_commit: <sha>
esb_tag: <optional>
checked_at: 2026-01-18T00:00:00+09:00
```
更新主体はツール CI（branding check 成功時）とする。配置はツール repo ルート固定。

## 参照メモ
- 下流運用フロー: `docs/downstream-branding-flow.md`

## 未決事項（次セッションで決める）
- ツール repo の versioning 方針（タグ / リリース / ESB commit 追従の運用）
- `branding.lock` の確定フォーマットと更新ルール
- CI 認証方式（PAT / GitHub App 等）

## 次のアクション案（新セッション）
1) ツール repo スケルトン作成と CI たたき台追加
2) branding.lock 仕様の確定
3) ESB -> ツール CI 連携の実装
4) ESB 本体から `tools/branding` と `mise` タスクを削除
