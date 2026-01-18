# 実装指示書: CLIの宣言的プロビジョニングモデルへの移行

## 1. コンテキストと目的 (Context & Objective)

### 現状の課題
現在、`esb up` コマンドにおけるリソース作成処理（`Provisioner`）は、**「SAMそのものの読み込みと解析」**と**「リソースの作成処理」**が密結合しています。
`provisioner` パッケージがファイルパス (`TemplatePath`) を受け取り、内部でパースからデプロイまでを一気通貫で行っているため、以下の問題があります。

1.  **責務の混在**: Provisionerが「ファイルIO」「パース」「API呼び出し」全てを知りすぎている。
2.  **テストの困難さ**: 特定のリソース定義に対する動作をテストするのに、毎回ファイルシステム上のテンプレートファイルが必要。
3.  **拡張性の欠如**: 将来的に「リモートコントロールプレーンへのマニフェスト送信」などを行う際、ファイル読み込み前提のロジックが邪魔になる。

### 変更の目的
「ファイルの読み込み・解析（Intent）」と「リソースの作成（Reconciliation）」を明確に分離します。
`Provisioner` は「何を作るべきか（Manifest）」を受け取り、「それを作る（Apply）」ことに専念させます。

---

## 2. アーキテクチャ設計 (Architecture)

### 2.1 データフローの変更

*   **AS-IS**: `App (up.go)` -> `Provisioner(FilePath)` -> [ Read File -> Parse ] -> `Create Resources`
*   **TO-BE**: `App (up.go)` -> [ Read File ] -> `Parser.Parse(Content)` -> `Manifest` -> `Provisioner.Apply(Manifest)` -> `Create Resources`

### 2.2 パッケージ構成の変更

新しい `manifest` パッケージを導入し、循環参照を防ぎながらデータ構造を共有します。

```text
cli/internal/
├── manifest/              # [NEW] 共通データ構造
│   └── resources.go       # ResourcesSpec, DynamoDBSpec, S3Spec, LayerSpec
├── generator/
│   ├── parser.go          # manifest.ResourcesSpec を返すように変更
│   └── parser_iface.go    # Parserインターフェース定義
├── provisioner/           # ファイル読み込みロジックを削除
│   ├── provisioner.go     # Apply(manifest.ResourcesSpec) に変更
│   └── ...
└── app/
    ├── up.go              # オーケストレーション (File Read -> Parse -> Apply)
    └── dependencies.go    # Parserの依存注入を追加
```

---

## 3. 実装ステップ (Implementation Steps)

コンパイルエラーを最小限に抑え、安全に移行するための手順です。

### Step 1: `manifest` パッケージの作成 (共通基盤)

`cli/internal/generator` にあるリソース定義を切り出します。

1.  **ディレクトリ作成**: `cli/internal/manifest`
2.  **型定義の移動**: `cli/internal/generator/parser.go` (および `parser_resources.go`) から以下の構造体を `cli/internal/manifest/resources.go` に移動します。
    *   `ResourcesSpec`
    *   `DynamoDBSpec`
    *   `S3Spec`
    *   `LayerSpec` (**重要**: `FunctionSpec` は `generator` に残すが、`FunctionSpec.Layers` フィールドのために `manifest` の import が必要になる)
3.  **依存関係の整理**:
    *   `manifest` パッケージ内で `github.com/poruru-code/aws-sam-parser-go/schema` を import します。
    *   将来的な依存排除のため、コメントに「依存排除時はここを独自定義に置き換える」旨を残してください。

### Step 2: `generator` パッケージの修正

`manifest` パッケージを利用するように修正します。

1.  **Import追加**: `cli/internal/generator/parser.go` 等で `.../cli/internal/manifest` を import。
2.  **型参照の更新**:
    *   `ParseResult` 構造体の `Resources` フィールドを `manifest.ResourcesSpec` に変更。
    *   `FunctionSpec` 構造体の `Layers` フィールドを `[]manifest.LayerSpec` に変更。
    *   `parser_resources.go` 内の生成ロジック (`parseOtherResources` 等) が `manifest` パッケージの構造体を生成するように修正。

### Step 3: `provisioner` ロジックの型差し替え (ロジック分離の準備)

まず内部ロジックが受け取る型を変更します。まだインターフェースは変更しません。

1.  **S3/DynamoDB実装の修正**:
    *   `cli/internal/provisioner/s3.go`: `provisionS3` 関数の引数を `[]generator.S3Spec` から `[]manifest.S3Spec` に変更。
    *   `cli/internal/provisioner/dynamodb.go`: `provisionDynamo` 関数の引数を `[]generator.DynamoDBSpec` から `[]manifest.DynamoDBSpec` に変更。

### Step 4: `app` 層のインターフェース変更 (Breaking Change)

ここが主要なリファクタリングポイントです。`app` パッケージと `provisioner` パッケージの境界線を変更します。

1.  **App層のインターフェース変更**:
    *   `cli/internal/app/provision.go`:
        *   `ProvisionRequest` 構造体を削除、または `TemplatePath` などのファイルパス依存フィールドを削除し、`Resources manifest.ResourcesSpec` を持つ形に再定義することを検討（今回は単純化のため、Applyメソッドのシグネチャ変更を推奨）。
        *   `Provisioner` インターフェースを以下のように変更：
            ```go
            type Provisioner interface {
                Apply(ctx context.Context, resources manifest.ResourcesSpec, composeProject string) error
            }
            ```

2.  **App層の依存関係追加**:
    *   `cli/internal/app/app.go` の `Dependencies` 構造体に `Parser generator.Parser` を追加。

### Step 5: `provisioner` 実装の刷新

`provisioner.Runner` をファイル読み込みから解放し、純粋な適用ロジックにします。

1.  **Runnerの修正** (`cli/internal/provisioner/provisioner.go`):
    *   `Parser` フィールドを削除（不要になるため）。
    *   `Provision(Request)` メソッドを廃止し、`Apply(...)` メソッドを実装。
    *   **削除**: `filepath.Abs`, `os.Stat`, `os.ReadFile`, `parser.Parse` などのコード。
    *   **維持**: ポート解決 (`resolvePort`)、クライアント取得、各リソースプロビジョニング関数 (`provisionDynamo`, `provisionS3`) への委譲。

### Step 6: `up` コマンドのオーケストレーション変更

削除したロジックを `up.go` に移植し、処理フローを再構築します。

1.  **`cli/internal/app/up.go` の修正**:
    *   テンプレートファイルのパス解決と読み込み処理を追加（元々 `provisioner` にあったロジック）。
    *   `deps.Parser.Parse` を呼び出して `manifest` を取得。
    *   `deps.Provisioner.Apply` を呼び出してリソース作成を実行。
    *   エラーハンドリング（「ファイルがない場合」など）が既存挙動と一致することを確認。

2.  **CLIのエントリーポイント修正 (`cli/cmd/esb/cli.go`)**:
    *   `manifest` 等の必要な新規 import を追加。
    *   `buildDependencies` 内で `generator.DefaultParser{}` を生成し、`Dependencies.Parser` に注入。
    *   `provisionerAdapter` は不要になる可能性が高いです。`provisioner.New` が返す `*Runner` がそのまま `app.Provisioner` (Applyメソッドを持つ) を満たすようになるため、直接代入できるようになります。

### Step 7: テストの修正

1.  **`cli/internal/app/up_test.go`**:
    *   `fakeProvisioner` を `Apply` メソッドを持つ形に修正。
    *   `fakeParser` (またはモック) を作成し、`Dependencies` に注入。
    *   「テンプレートファイルが存在しない場合」のエラーテストケースを追加（以前は `provisioner` 内でチェックしていたが、`up.go` 側でチェックするようになったため）。

2.  **`cli/internal/provisioner/provisioner_test.go`**:
    *   テストデータ生成部分を `manifest` パッケージを使用するように修正。
    *   ファイルIOのモックなどが不要になり、テストがシンプルになるはずです。

---

## 4. 検証 (Verification)

### 4.1 ビルド確認
各ステップごとに `go build ./cli/...` を実行し、参照エラーがないことを確認します。

### 4.2 テスト実行
```bash
go test ./cli/...
```
特に `up` コマンド周りのテストが、リファクタリング前後で同じカバレッジと結果を維持していることを確認します。

### 4.3 動作確認 (Manual)
1.  正常系: `esb up` が通り、DynamoDB/S3 ローカルリソースが作成されること。
2.  異常系: テンプレートファイル名が存在しないパスを指定した場合 (`esb up -t invalid.yaml`)、適切なエラーメッセージが表示されること。

---

## 補足: リスクと対応

*   **循環参照**: `generator` と `provisioner` が互いに参照しないように注意してください。両者は `manifest` (および `schema` 等の外部ライブラリ) にのみ依存すべきです。
*   **インターフェース不整合**: `app` パッケージでの `Provisioner` インターフェース定義変更は、`test` 内のモックにも波及します。変更範囲が広いため、Step 7のテスト修正は丁寧に行ってください。
