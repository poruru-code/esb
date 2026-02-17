<!--
Where: docs/branding-generator.md
What: Pointer to branding documentation in tool repo.
Why: Keep branding operations documented where the tool lives.
-->
# ブランディング運用（外部ツール）

ブランディング生成と下流リポジトリ向け配布手順は、`esb-branding-tool` 側を source of truth とします。

- https://github.com/poruru-code/esb-branding-tool/blob/main/docs/branding-flow.md

本リポジトリでは CLI コマンド名は `esb` 固定です。
ブランド変更は本体で直接行わず、外部ツールで生成・適用してください。

---

## Implementation references
- `tools/bootstrap/README.md`
