<!--
Where: docs/runtime-identity-contract.md
What: Shared runtime identity contract for stack-derived naming.
Why: Keep CLI/Agent module-local constants decoupled while preserving naming compatibility.
-->
# Runtime Identity Contract

## Scope
このドキュメントは、実行中の compose stack 文脈から runtime 名称を導出する契約を定義します。  
対象は `services/agent` の runtime 命名（container 名、label、namespace、CNI 名、image 接頭辞）です。

## Input と優先順位
brand slug は次の順序で 1 つに解決します。

1. `ESB_BRAND_SLUG`（明示指定）
2. `PROJECT_NAME` + `ENV`（末尾 `-<env>` / `_<env>` を除去）
3. `CONTAINERS_NETWORK`（末尾 `-external` / `_<env>` を除去）
4. fallback `esb`

## Normalization
- 小文字化
- 英数字以外は `-` に変換
- 連続した区切りは 1 つに圧縮
- 先頭/末尾の `-` は除去
- 空になった場合は `esb`

## Derived Values
| 項目 | ルール |
| --- | --- |
| `BrandSlug` | 解決済み brand |
| `RuntimeNamespace` | `<brand>` |
| `RuntimeCNIName` | `<brand>-net` |
| `RuntimeCNIBridge` | `esb0`（runtime-node forwarding 互換のため固定） |
| `RuntimeContainerPrefix` | `<brand>` |
| `ImagePrefix` | `<brand>` |
| `EnvPrefix` | `<brand>` を大文字化し `-` を `_` へ変換 |
| `LabelPrefix` | `com.<brand>` |
| `RuntimeLabelFunction` | `<brand>_function` |
| `RuntimeLabelCreatedBy` | `created_by` |
| `RuntimeLabelCreatedByValue` | `<brand>-agent` |
| `RuntimeLabelEnv` | `<brand>_env` |
| `RuntimeLabelKind` | `com.<brand>.kind` |
| `RuntimeLabelOwner` | `com.<brand>.owner` |
| `RuntimeResolvConfPath` | `/run/containerd/<brand>/resolv.conf` |

## Tag Compatibility
既定タグ解決は `<EnvPrefix>_TAG` を優先し、未設定時は `ESB_TAG`、さらに未設定時は `latest` を使います。

## Compose Injection Contract
stack から安定して brand を導出するため、Agent には最低限以下を渡します。

- `PROJECT_NAME`
- `ENV`
- `CONTAINERS_NETWORK`

## Source of Truth
- `services/agent/internal/identity/stack_identity.go`
- `services/agent/internal/runtime/constants.go`
- `services/agent/internal/runtime/image_naming.go`
- `services/agent/cmd/agent/main.go`
