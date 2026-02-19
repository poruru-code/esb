<!--
Where: services/runtime-node/docs/devmapper.md
What: devmapper pool requirements and setup guidance.
Why: runtime-node expects an existing thin-pool; it never creates one.
-->
# devmapper（Firecracker / containerd）

## 前提
runtime-node は **devmapper pool を自動作成しません**。起動時に pool が存在しない場合は、
`ensure_devmapper_ready` がエラーで停止します。

## 期待される挙動
- `DEVMAPPER_POOL` が未設定: devmapper を使わずに起動
- `DEVMAPPER_POOL` が設定済み: **既存 pool を必須**として検証

## 事前準備（開発用）
thin-pool はホスト側で事前作成してください（runtime-node は作成しません）。
運用方法はホスト標準手順に従ってください。

## 関連環境変数
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `DEVMAPPER_POOL` | (空) | 使用する thin-pool 名 |
| `DEVMAPPER_DIR` | `/var/lib/containerd/devmapper2` | メタ/データファイルの場所 |
| `DEVMAPPER_DATA_SIZE` | `10G` | データデバイスサイズ |
| `DEVMAPPER_META_SIZE` | `2G` | メタデバイスサイズ |
| `DEVMAPPER_UDEV` | `0` | `1` のとき udev を使用 |

---

## Implementation references
- `services/runtime-node/entrypoint.common.sh`
