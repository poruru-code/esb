<!--
Where: services/provisioner/docs/README.md
What: Entry point for Provisioner subsystem documentation.
Why: Keep resource provisioning behavior close to its implementation.
-->
# Provisioner ドキュメント

Provisioner は **runtime-config の `resources.yml`** を読み取り、S3/DynamoDB 互換サービスへ
リソースを作成するバッチコンテナです。deploy フローから起動され、基本的に **一度実行して終了**します。

## まず読む順序
1. [アーキテクチャ](./architecture.md)
2. [resources.yml 仕様](./resources-manifest.md)
3. [設定（環境変数）](./configuration.md)

## 目的別ガイド
| 目的 | 参照先 |
| --- | --- |
| 実行フローを知りたい | [architecture.md](./architecture.md) |
| deploy から渡る入力仕様を知りたい | [resources-manifest.md](./resources-manifest.md) |
| 接続設定を確認したい | [configuration.md](./configuration.md) |

## 関連
- System-level: [docs/spec.md](../../../docs/spec.md)

---

## Implementation references
- `services/provisioner/src/main.py`
- `docker-compose.docker.yml`
- `docker-compose.containerd.yml`
