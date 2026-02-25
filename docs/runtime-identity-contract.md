<!--
Where: docs/runtime-identity-contract.md
What: Shared runtime identity contract for stack-derived naming.
Why: Keep producer/Agent module-local constants decoupled while preserving naming compatibility.
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

上記のいずれでも解決できない場合は hard fail します（fallback しません）。

## Normalization
- 小文字化
- 英数字以外は `-` に変換
- 連続した区切りは 1 つに圧縮
- 先頭/末尾の `-` は除去
- 空になった場合は未解決扱い（hard fail）

## Derived Values
| 項目 | ルール |
| --- | --- |
| `BrandSlug` | 解決済み brand |
| `RuntimeNamespace` | `<brand>` |
| `RuntimeCNIName` | `<brand>-net` |
| `RuntimeCNIBridge` | `esb-<brand4><hash6>` |
| `RuntimeCNISubnet` | 決定論的な `10.x.y.0/23` |
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

補足:
- `RuntimeCNISubnet` は `10.88.x.x` 帯を除外した `/23` スロットから派生されます。
- Agent は `CNI_SUBNET` 未指定時に既存 CNI 設定の subnet 使用状況を確認し、衝突時は同一 hash 空間の次スロットへ進めます（決定論維持）。
- 有限 IPv4 スロット上の決定論マッピングのため理論上の衝突可能性は残りますが、従来の `/20` 派生よりスロット空間を拡張し、かつローカル衝突は上記 probe で回避します。

## Tag Compatibility
既定タグ解決は `<EnvPrefix>_TAG` を優先し、未設定時は `latest` を使います。

## Compose Injection Contract
stack から安定して brand を導出するため、Agent には最低限以下を渡します。

- `ENV`
- `CONTAINERS_NETWORK`

`PROJECT_NAME` は任意入力です（あれば導出優先度を上げるために使用）。
`ENV` と `CONTAINERS_NETWORK` の両方が欠落した場合は Agent 起動時に失敗します。

## Source of Truth
- `services/agent/internal/identity/stack_identity.go`
- `services/agent/internal/runtime/constants.go`
- `services/agent/internal/runtime/image_naming.go`
- `services/agent/cmd/agent/main.go`
