# `esb up` コマンド

## 概要

`esb up` コマンドは、ローカルサーバーレス環境を起動します。必要に応じて成果物のビルド、Docker Composeによるコンテナ起動、およびSAMテンプレートで定義されたローカルAWSリソース（DynamoDB, S3）のプロビジョニングを行い、ライフサイクルを管理します。

## 使用方法

```bash
esb up [flags]
```

### フラグ

| フラグ | 短縮形 | 説明 |
|--------|--------|------|
| `--env` | `-e` | ターゲット環境 (例: local)。デフォルトは最後に使用された環境です。 |
| `--build` | | 起動前にイメージを再ビルドします。 |
| `--reset` | | 環境をリセットします (`down --volumes` + `build` + `up` と同等)。 |
| `--yes` | `-y` | `--reset` 時の確認プロンプトをスキップします。 |
| `--detach` | `-d` | コンテナをバックグラウンドで実行します (デフォルト: true)。 |
| `--wait` | `-w` | ゲートウェイの準備完了を待機します。 |
| `--env-file` | | カスタム `.env` ファイルへのパスを指定します。 |

## 実装詳細

コマンドのロジックは `cli/internal/app/up.go` に実装されています。`Upper`, `Builder`, `Downer`, `Provisioner`, `PortDiscoverer` といった複数のインターフェースに依存する高レベルなオーケストレーターとして機能します。

### ワークフローステップ

1. **コンテキスト解決**: アクティブな環境とプロジェクトルートを決定します。
2. **リセット (オプション)**: `--reset` が指定された場合、ボリューム削除を有効にして `Downer.Down` を呼び出します。
3. **認証**: `auth.json` の存在を確認し、不足している場合はデフォルトの認証情報を生成します。
4. **ビルド (オプション)**: `--build` または `--reset` が指定された場合、`Builder` を呼び出してイメージを再生成します。
5. **Docker Compose Up**: `Upper.Up` を呼び出してコンテナ (Gateway, Agentなど) を起動します。
6. **ポート検出**: 動的に割り当てられたポートがあればスキャンし、永続化します。
7. **プロビジョニング**: `template.yaml` を解析し、`Provisioner` を介してローカルリソース (テーブル、バケット) を設定します。
8. **待機 (オプション)**: `--wait` が指定された場合、Gatewayのヘルスエンドポイントが準備完了になるまでポーリングします。

## シーケンス図

```mermaid
sequenceDiagram
    participant CLI as esb up
    participant Downer as Downer
    participant Builder as Builder
    participant Upper as Upper (Compose)
    participant Docker as Docker Daemon
    participant Provisioner as Provisioner
    participant Waiter as GatewayWaiter

    CLI->>CLI: コンテキスト解決

    opt --reset
        CLI->>Downer: Down(volumes=true)
        Downer->>Docker: コンテナ停止 & ボリューム削除
    end

    CLI->>CLI: 認証情報の確認 (EnsureAuthCredentials)

    opt --build OR --reset
        CLI->>Builder: Build(BuildRequest)
    end

    CLI->>Upper: Up(UpRequest)
    Upper->>Docker: compose up -d

    CLI->>CLI: ポート検出と保存 (DiscoverAndPersistPorts)

    CLI->>Provisioner: Provision(ProvisionRequest)
    Provisioner->>Provisioner: SAMテンプレート解析
    Provisioner->>Docker: DynamoDB/S3設定 (AWS SDK)

    opt --wait
        CLI->>Waiter: Wait(Context)
        loop 準備完了まで
            Waiter->>Docker: ヘルスチェック
        end
    end

    CLI-->>CLI: 成功メッセージ & ポート表示
```
