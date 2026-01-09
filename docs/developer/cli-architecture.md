# `esb` CLI アーキテクチャ

`esb` CLI (`cli/cmd/esb`) とジェネレータ (`cli/internal/generator`) は、SAM テンプレート → Compose 運用の全工程を完結させるための基盤です。この文書では CLI 利用者向けの設計、処理フロー、スキーマ追加時の手順を収録しています。

## コンポーネント概要

- `cli/cmd/esb`: `kong` ベースの CLI エントリポイント。`app.Run` を呼び出して依存 (`Dependencies`) を注入し、`build`/`up`/`down`/`logs`/`stop`/`prune`/`env`/`project` などのコマンドを実行します。
- `cli/internal/app`: コマンドごとのリクエスト構造とステートマシン (`resolveCommandContext`, `app.Context`) を持ち、`compose` へ依頼するための `BuildRequest`/`UpRequest`/`StopRequest` を組み立てます。
- `cli/internal/generator`: Parser/Renderer で `template.yaml` を `functions.yml`/`routing.yml`・Dockerfile に変換し、`go_builder` で Docker イメージや Compose 設定まで進みます。
- `cli/internal/compose`: `docker compose` を呼び出すユーティリティで、`ResolveComposeFiles` により `docker-compose.yml` + mode 固有ファイルを選びます。
- `cli/internal/state`: `generator.yml` や `global_config` を読み込んで `ESB_ENV`, `ESB_PROJECT_NAME`, `ESB_MODE` を管理し、すべてのコマンドが共通の状態知識を共有します。

## クラス図

```mermaid
classDiagram
    class CLI {
        +Run
    }
    class App {
        +runBuild
        +runUp
        +runDown
    }
    class Generator {
        +GenerateFiles
    }
    class Compose {
        +Build
        +Up
        +Logs
    }
    class State
    CLI --> App
    App --> Generator
    App --> Compose
    Generator --> State
```

## ビルド・起動フロー

```mermaid
flowchart TD
    A[esb build --env <env>] --> B[resolve generator.yml & template]
    B --> C[cli/internal/generator/parser]
    C --> D[staging .esb/functions/<fn>]
    D --> E["docker compose build (esb-lambda-base + functions)"]
    E --> F[esb up --env <env>] --> G["docker compose up control (gateway/agent/runtime)"]
    G --> H[esb logs / stop / prune] --> I[docker compose logs/stop/down]
```

## generator.yml とステートマシン

`generator.yml` は `app.tag`, `PathsConfig`, `Environments` を含みます。CLI は次のように管理します。

1. `esb init` で `generator.yml` を生成し `environments` 配列を初期化。
2. `esb env add/use`・`esb project use` で `generator.yml` と `global_config.toml` を更新し、`cli/internal/state` が `Context` を解決。
3. `build`/`up`/`down` では `resolveCommandContext` で `ESB_ENV`/`ESB_PROJECT_NAME` などを設定し、`cli/internal/generator/go_builder` が `GenerateOptions` を組み立てる。
4. `cli/internal/app` の `applyModeEnv` で `ESB_MODE`, `ESB_IMAGE_TAG` を注入し、Compose に渡します。

## Schema 追加・更新手順

1. `cli/internal/generator/schema/sam.schema.json` に必要なフィールド/定義を追加し、`sam.schema.json` の `$ref` 参照を使って `sam_generated.go` を再生成（`go-jsonschema` を使用）。  
2. `sam_generated.go` を再生成後、`Parser` 側（`parser.go`）で新しいプロパティを `FunctionSpec` 等にマッピング。数値/文字列混在は `asString`/`asIntPointer` で正規化。  
3. `validator.go` で `gojsonschema` を使ったバリデーションを追加し、`.tmp/template.yml` や `e2e/fixtures/template.yaml` などで `validateSAMTemplate` テストを回して新しいフィールドを含むか確認。  
4. `cli/internal/generator/templates` の Dockerfile/functions/routing テンプレートに必要な変数（`Functions`, `Events`）を追加し、`renderer.go` がロジックを反映。`renderer_test.go` を用意して YAML 出力を検証。  
5. このドキュメントや `docs/generator-architecture.md` を更新し、新しいフィールド一覧と検証手順を記録。E2E を `uv run python e2e/run_tests.py --reset --parallel` で再実行して生成/Compose が通ることを確認。

## 検証ポイント

- `cd cli && go test ./...` でユニットを通す。`cli/internal/generator` への `validator_test` を含める。  
- `uv run python e2e/run_tests.py --parallel --reset` で `e2e-docker`/`e2e-containerd` 両プロファイルの 39 テストが通るかを確認。  
- `esb build --env <env>` → `esb up --env <env>` → `esb logs/stop/prune` の組み合わせで `docker compose` の状態遷移が正しいか確認。

## ステートマシン

以下は CLI の内部状態遷移を示す図です。`resolveCommandContext` で取得した `Context` が `State` を基にしており、`generator.yml`/`.esb` の状態とコマンドが同期します。

```mermaid
stateDiagram
    [*] --> Uninitialized
    Uninitialized --> Initialized : esb init
    Initialized --> Building : esb build
    Building --> Up : build success
    Up --> Up : esb up --build
    Up --> Stopped : esb stop/prune
    Stopped --> Building : esb build
    Stopped --> Up : esb up
    Up --> Resetting : esb reset
    Resetting --> Initialized : reset complete
    Building --> Failed : build error
    Failed --> Stopped : esb stop
    Failed --> Resetting : esb reset
```

この図はエラーや再実行時の遷移も含み、`esb reset`/`esb prune` のような「完全リセット」操作も明示しています。変更時は `cli/internal/state/context.go` と `app` パッケージ内でこのステートを追跡しているか確認してください。

このドキュメントは `esb` CLI の開発者向けに、クラス図・処理フロー・スキーマ更新手順をまとめたものです。常に `cli/internal/generator`、`cli/internal/compose`、`cli/internal/state` が同期していることを意識して変更を加えてください。
