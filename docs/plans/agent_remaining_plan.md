# Agent 残件対応計画

対象: `docs/reports/agent_architecture_review.md` の未解決項目  
前提: コンテナ分離は不可（資源制御・防御は可能）

## 目的
- レビュー残件を優先度順に整理し、対応順と判断ポイントを明確化する。

## 進め方
- 仕様判断が必要な項目は先に合意し、実装/テスト/ドキュメント更新までを1セットで完了させる。
- 影響範囲が大きいものは `docs/` に設計メモを残す。

## 優先度 P1（セキュリティ/安定性）
- P1-1: gRPC セキュリティ方針の確定と実装  
  - 反映先: `services/agent/cmd/agent/main.go`  
  - 決めること: 既定で mTLS を必須にするか、または既定 OFF + 強警告のまま継続するか。  
  - 受け入れ条件: 反射 API の有効/無効が環境変数で明確に制御でき、起動時の警告/ログが適切。
- P1-2: `InvokeWorker` レスポンスのサイズ上限 ✅ **完了**
  - 反映先: `services/agent/internal/api/server.go`  
  - 受け入れ条件: 上限値が設定可能で、超過時は明確なエラーを返す。
  - **実装済み**: `io.LimitReader` + `AGENT_INVOKE_MAX_RESPONSE_SIZE` 環境変数（デフォルト10MB）
- P1-3: 资源制御（CPU/メモリ上限）  
  - 反映先: `services/agent/internal/runtime/containerd/*`, `services/agent/internal/runtime/docker/*`  
  - 受け入れ条件: デフォルト上限が設定可能で、無制限を選べる場合は明示設定が必要。
  - **Note**: containerd のメモリ制限は `runtime.go:286` で `oci.WithMemoryLimit` により実装済み。残件は CPU 制限の追加と Docker 側の資源制御。
- P1-4: insecure registry 利用可否の整理  
  - 反映先: `services/agent/internal/runtime/containerd/image.go`  
  - 決めること: HTTPS 強制を維持するか、環境変数で HTTP を許可するか。  
  - 受け入れ条件: 期待したレジストリ接続方式が選択できる。

## 優先度 P2（互換性/実行時安定）
- P2-1: Docker で IP 未確定時の扱い ✅ **完了**
  - 反映先: `services/agent/internal/runtime/docker/runtime.go`  
  - 受け入れ条件: リトライ/待機/エラー返却が一貫し、呼び出し側が原因を判別できる。
  - **実装済み**: 指数バックオフ付きリトライ（5回、100ms〜1.6s）、IP未取得時は明示的エラー
- P2-2: Pause/Resume 未実装の扱い ✅ **完了**
  - 反映先: `services/agent/internal/runtime/docker/runtime.go`, `services/agent/internal/api/server.go`  
  - 受け入れ条件: `codes.Unimplemented` を返し、クライアントが判別可能。
  - **実装済み**: Docker runtime が `codes.Unimplemented` を返すよう変更
- P2-3: コンテナ名のサニタイズ/衝突回避  
  - 反映先: `services/agent/internal/runtime/containerd/runtime.go`, `services/agent/internal/runtime/docker/runtime.go`  
  - 受け入れ条件: 名前制約に準拠し、表示名はラベルで保持できる。

## 優先度 P3（運用性/可観測性）
- P3-1: ログ出力の統一と DEBUG 制御 ✅ **完了**
  - 反映先: `services/agent/internal/runtime/*`  
  - 受け入れ条件: ログ出力が一貫し、機微情報はデフォルトで抑制。
  - **実装済み**: `log/slog` を導入し、`main.go` を移行。`AGENT_LOG_LEVEL` で制御可能。
- P3-2: `InvokeWorker` 成功/失敗とレイテンシの計測 ✅ **完了**
  - 反映先: `services/agent/internal/api/server.go`  
  - 受け入れ条件: 計測が取得可能で、負荷調査に使える。
  - **実装済み**: `go-grpc-middleware/v2` のロギングインターセプターを導入。全 gRPC メソッドのレイテンシと結果を構造化ログ出力。
- P3-3: gRPC Health の導入 ✅ **完了**
  - 反映先: `services/agent/cmd/agent/main.go`  
  - 受け入れ条件: readiness/liveness が gRPC で判定可能。
  - **実装済み**: `grpc/health` サービスを登録し `SERVING` を返却するよう実装。
- P3-4: `LastUsedAt` 更新タイミングの見直し ✅ **完了**
  - 反映先: `services/agent/internal/runtime/containerd/runtime.go`, `services/agent/internal/runtime/docker/runtime.go`
  - 受け入れ条件: 実際の利用に追従する。
  - **実装済み**: `Touch()` メソッドをインターフェースに追加し、`InvokeWorker` 開始時に呼び出すよう実装。Docker 版にも `accessTracker` を導入し、正確な GC 判定を可能にした。

## 優先度 P4（保守性/一貫性）
- P4-1: `ContainerState.Status` の正規化 ✅ **完了**
  - 反映先: `services/agent/internal/runtime/interface.go` + runtime 実装  
  - 受け入れ条件: どの runtime でも共通の定数（`RUNNING`, `STOPPED` 等）を返すように統一。
  - **実装済み**: `interface.go` に共通定数を定義し、Docker/Containerd 双方の `List` 実装でマッピングするように修正。
- P4-2: 設定デフォルトの一元管理  
  - 反映先: `services/agent/cmd/agent/main.go`, `services/agent/internal/api/server.go`  
  - 受け入れ条件: 既定値が一箇所に集約され、テストが簡潔。
- P4-3: `PortAllocator` の削除または活用方針決定 ✅ **完了**
  - 反映先: `services/agent/internal/runtime/containerd/port_allocator.go`  
  - 受け入れ条件: 未使用コードが残らない。
  - **実装済み**: `port_allocator.go` および `port_allocator_test.go` を削除
- P4-4: Docker `Metrics` の扱い  
  - 反映先: `services/agent/internal/runtime/interface.go`, `services/agent/internal/runtime/docker/runtime.go`  
  - 受け入れ条件: capability を明示 or Docker 側実装。

## テスト/ドキュメント
- 対応項目ごとに UT を追加し、必要に応じて `docs/` に運用・設定の追記を行う。

