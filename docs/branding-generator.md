<!--
Where: docs/branding-generator.md
What: Pointer to branding documentation in tool repo.
Why: Keep branding operations documented where the tool lives.
-->
# ブランディング運用（外部ツール）

ブランディング生成と下流リポジトリ向け配布手順は、`esb-branding-tool` 側を source of truth とします。

- https://github.com/poruru-code/esb-branding-tool/blob/main/docs/branding-flow.md
- baseline棚卸し（固定スナップショット）: `docs/esb-usage-dependency-inventory.md`
- 推奨実行キー: `TAG`, `REGISTRY`, `SKIP_GATEWAY_ALIGN`, `REGISTRY_WAIT`, `CLI_BIN`, `META_REUSE`
- 運用方針: Phase2 以降は `ESB_*` 互換キーを使わず、上記の中立キーのみを使用する

本リポジトリでは `config/defaults.env` の `CLI_CMD=esb` を基準値として維持します。
ブランド変更は本体で直接行わず、外部ツールで生成・適用してください。

---

## Implementation references
- `config/defaults.env`
