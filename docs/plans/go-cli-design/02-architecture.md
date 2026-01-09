# アーキテクチャ

## データモデル

```
Source Project (入力・変更しない)     Output Directory (出力・設定可能)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━     ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
/path/to/sam-project/                /path/to/output/.esb/
├── template.yaml    ← SAM          ├── default/          ← 環境1
├── generator.yml    ← ESB設定      │   ├── functions/
├── functions/       ← 元コード     │   │   ├── hello/
│   ├── hello/                      │   │   │   ├── Dockerfile
│   │   └── app.py                  │   │   │   ├── src/        ← コピー
│   └── world/                      │   │   │   └── sitecustomize.py
│       └── app.py                  │   │   └── world/
└── layers/                         │   └── config/
    └── common/                     │       ├── functions.yml
                                    │       └── routing.yml
                                    └── staging/          ← 環境2
                                        ├── functions/
                                        └── config/
```

**ポイント:**
- ソースプロジェクトは template.yaml の場所で特定
- `generator.yml` は template.yaml と同じディレクトリに保存
- `output_dir` は generator.yml の `paths.output_dir` で設定可能
- `environments` は環境名と runtime mode の対応表
- ビルド時に関数コードは output_dir にコピーされる
- 環境ごとに独立した出力ディレクトリ

## generator.yml (ESB 設定)

```yaml
# /path/to/sam-project/generator.yml
app:
  name: my-serverless-app
  tag: default

environments:
  default: docker
  staging: containerd
  production: firecracker

paths:
  sam_template: ./template.yaml     # 相対パス
  output_dir: .esb/                 # 出力先 (設定可能)

parameters:
  S3BucketName: my-bucket
  TableName: my-table
```

## プロジェクト登録

複数の SAM プロジェクトを管理するため、グローバル設定を `~/.esb/config.yaml` に保存:

```yaml
# ~/.esb/config.yaml
version: 1
active_project: my-serverless-app
active_environments:
  my-serverless-app: staging
  another-project: default
projects:
  my-serverless-app:
    path: /path/to/sam-project      # template.yaml の親ディレクトリ
    last_used: 2026-01-08T23:45:00+09:00
  another-project:
    path: /path/to/another-project
    last_used: 2026-01-08T21:30:00+09:00
```

## ディレクトリ構成

```
cmd/esb/
├── main.go           # エントリーポイント
├── cli.go            # CLI 構造体 (Kong)
├── context.go        # 実行コンテキスト
├── init.go           # esb init
├── build.go          # esb build
├── up.go             # esb up
├── down.go           # esb down
├── prune.go          # esb prune
├── status.go         # esb status
├── env.go            # esb env
├── project.go        # esb project
├── logs.go           # esb logs
├── stop.go           # esb stop

internal/
├── state/            # ステートマシン
│   ├── state.go      # 状態定義
│   └── detector.go   # 状態検出
├── config/           # 設定管理
│   ├── config.go     # ポート/ネットワーク計算
│   ├── project.go    # プロジェクト設定
│   └── global.go     # グローバル設定 (~/.esb/config.yaml)
├── compose/          # Docker Compose 操作
│   └── compose.go
├── generator/        # SAM テンプレート処理
│   ├── parser.go
│   └── renderer.go
├── provisioner/      # DynamoDB/S3 作成
│   └── provisioner.go
├── cert/             # 証明書管理
│   └── cert.go
└── log/              # ロギング
    └── log.go
```

## 依存ライブラリ

```go
require (
    github.com/alecthomas/kong v1.6.0     // CLI フレームワーク
    github.com/docker/docker v27.0.0       // Docker クライアント
    github.com/aws/aws-sdk-go-v2 v1.30.0   // DynamoDB/S3
    gopkg.in/yaml.v3 v3.0.1                // YAML パース
    github.com/fatih/color v1.17.0         // カラー出力
)
```
