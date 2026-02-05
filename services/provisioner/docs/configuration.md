<!--
Where: services/provisioner/docs/configuration.md
What: Provisioner environment variables and runtime expectations.
Why: Make deployment wiring explicit and easy to validate.
-->
# Provisioner 設定（環境変数）

## 入力ファイル
| 項目 | 値 |
| --- | --- |
| resources.yml | `/app/runtime-config/resources.yml` |

## サービス接続
| 変数 | 例 | 説明 |
| --- | --- | --- |
| `DYNAMODB_ENDPOINT` | `http://database:8000` | DynamoDB (Scylla Alternator) の接続先 |
| `S3_ENDPOINT` | `http://s3-storage:9000` | S3 (RustFS) の接続先 |
| `VICTORIALOGS_URL` | `http://victorialogs:9428` | ログ送信先（sitecustomize により利用） |

## AWS 認証
| 変数 | 例 | 説明 |
| --- | --- | --- |
| `AWS_DEFAULT_REGION` | `us-east-1` | boto3 の既定リージョン |
| `AWS_ACCESS_KEY_ID` | `esb` | RustFS のアクセスキー |
| `AWS_SECRET_ACCESS_KEY` | `esbsecret` | RustFS のシークレット |

## 実行時動作
- `resources.yml` が存在しない場合は **警告して終了**します。
- DynamoDB / S3 のどちらも定義がなければ **スキップ**します。

---

## Implementation references
- `services/provisioner/src/main.py`
- `services/provisioner/Dockerfile`
- `docker-compose.containerd.yml`
