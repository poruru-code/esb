# S3 Lifecycle Configuration サポート実装計画

現在の `esb build` および `esb provision` プロセスを拡張し、SAMテンプレート (`AWS::S3::Bucket`) 内の `LifecycleConfiguration` プロパティをサポートします。

## 概要

1.  **Parser**: `cli/internal/generator/parser.go` を修正し、S3バケットの `LifecycleConfiguration` プロパティを読み取る。
2.  **Provisioner**: `cli/internal/provisioner` パッケージを拡張し、読み取ったライフサイクル設定を MinIO/S3 に適用する。

## 詳細プラン

### 1. Parser の拡張 (`cli/internal/generator`)

#### [MODIFY] `parser.go`
*   `S3Spec` 構造体に `LifecycleConfiguration` フィールドを追加します。型は柔軟性を持たせるため `any` (または `map[string]any` 相当) とし、後段の Provisioner で詳細な変換を行います。

```go
type S3Spec struct {
    BucketName             string
    LifecycleConfiguration any // 追加
}
```

*   `ParseSAMTemplate` 関数内の `AWS::S3::Bucket` ケース (`case "AWS::S3::Bucket":`) を更新し、`Properties` から `LifecycleConfiguration` を取得して `S3Spec` にセットします。

### 2. Provisioner の拡張 (`cli/internal/provisioner`)

#### [MODIFY] `s3.go`
*   `S3API` インターフェースに `PutBucketLifecycleConfiguration` メソッドを追加します。

```go
type S3API interface {
    ListBuckets(ctx context.Context) ([]string, error)
    CreateBucket(ctx context.Context, name string) error
    PutBucketLifecycleConfiguration(ctx context.Context, name string, rules any) error // 追加
}
```

*   `provisionS3` 関数を更新します。バケットが作成された後（または既に存在する場合でも）、`bucket.LifecycleConfiguration` が `nil` でなければ `PutBucketLifecycleConfiguration` を呼び出して設定を適用します。

#### [MODIFY] `aws_clients.go`
*   `awsS3Client` 構造体に `PutBucketLifecycleConfiguration` メソッドを実装します。
*   **マッパー関数の実装**:
    *   Parser から渡された `Parsed` (Raw Map/Struct) データ構造を、AWS SDK v2 の `s3.PutBucketLifecycleConfigurationInput` および `types.BucketLifecycleConfiguration` に変換するヘルパー関数 (`buildAWSLifecycleConfigurationInput` 等) を実装します。
    *   YAML/JSON からの `Rules` リスト、`Filter`、`Status`、`Expiration`、`Transitions` などのフィールドを再帰的にマッピングする必要があります。

### 3. AWS SDK の利用
*   `github.com/aws/aws-sdk-go-v2/service/s3` と `types` パッケージを利用して型安全に設定を構築します。

## 検証
*   `tests/test_s3_lifecycle.yml` (仮) のようなテスト用SAMテンプレートを作成。
*   `LifecycleConfiguration` を含むバケットを定義。
*   `esb provision` を実行。
*   MinIO (または S3) に対して `aws s3api get-bucket-lifecycle-configuration --bucket ...` 等を実行し、正しく設定が反映されているか確認する。
