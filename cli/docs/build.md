# ビルドパイプライン（内部）

## 概要
`esb` CLI は deploy 時に内部のビルドパイプラインを実行します。SAM テンプレート
(`template.yaml`) を解析し、各 Lambda 関数用の Dockerfile と `functions.yml` / `routing.yml`
を生成し、対応する Docker イメージをビルドします。

> NOTE: `esb build` コマンドは現在ユーザー向けには公開していません。
> このドキュメントは deploy で利用される内部ビルド処理の説明です。

## 実装詳細
CLI アダプタは `cli/internal/command/deploy.go`、オーケストレーションは
`cli/internal/usecase/deploy/deploy.go` が担当します。実際のビルド処理は
`cli/internal/infra/build/go_builder.go` (GoBuilder) に委譲されます。

### 主要コンポーネント

- **`DeployWorkflow`**: ランタイム環境適用と Builder 呼び出しを行うオーケストレーター。
- **`BuildRequest`**: CLI から Workflow に渡される入力 DTO（`build.BuildRequest`）。
- **`ApplyRuntimeEnv`**: `ENV_PREFIX` 付きの環境変数を適用。
- **`UserInterface`**: 成功メッセージの出力（互換のため `LegacyUI` を使用）。
- **`GoBuilder`**: 関数イメージ生成・ビルドを担当する実装。
  - **`Generate`**: ビルド成果物 (Dockerfile, `functions.yml`, `routing.yml`) を出力ディレクトリに生成。
  - **`BuildCompose`**: コントロールプレーンのイメージ (Gateway, Agent) をビルド。
  - **`Runner`**: ベースイメージと関数イメージに対して `docker build` を実行。

### ビルドロジック

1. **入力解決**: `template` / `env` / `mode` / `output` を解決します。省略された値は対話で補完。
2. **ランタイム環境適用**: `RuntimeEnvApplier` が `ENV_PREFIX` 付きの変数を適用。
3. **パラメータ入力**: SAM テンプレートの `Parameters` が定義されている場合、対話的に入力を求めます。
4. **コード生成**:
   - `template.yaml` を解析
   - Dockerfile と `functions.yml` / `routing.yml` を生成
   - 出力ディレクトリ（`output/<env>/` または指定パス）に出力
5. **イメージビルド**:
   - ローカルレジストリ稼働確認 (必要時)
   - ベースイメージ (共有レイヤー) をビルド
   - 各関数イメージをビルド
   - コントロールプレーン (Gateway/Agent/Provisioner/Runtime Node) を Buildx Bake でビルド

## シーケンス図

```mermaid
sequenceDiagram
    participant CLI as esb deploy
    participant WF as DeployWorkflow
    participant Env as RuntimeEnvApplier
    participant Builder as GoBuilder
    participant Generator as Build Pipeline
    participant Docker as Docker Daemon
    participant Registry as Local Registry

    CLI->>WF: Run(BuildRequest)
    WF->>Env: Apply(Context)
    WF->>Builder: Build(BuildRequest)
    Builder->>Builder: SAMテンプレート読み込み
    Builder->>Generator: Generate(config, options)
    Generator-->>Builder: FunctionSpecs (関数リスト)

    Builder->>Builder: 設定ファイルのステージング (.env 等)

    opt 外部レジストリ
        Builder->>Registry: 稼働確認 (Ensure Running)
    end

    Builder->>Docker: ベースイメージのビルド

    loop 各関数に対して
        Builder->>Docker: 関数イメージのビルド
    end

    Builder->>Docker: コントロールプレーンのビルド (Gateway/Agent)

    Docker-->>Builder: 成功
    Builder-->>WF: 成功
    WF-->>CLI: 成功
```
