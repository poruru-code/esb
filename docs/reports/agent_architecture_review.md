# Agent 実装アーキテクチャレビュー（5 Passes）

対象: `services/agent`
制約: コンテナ分離は不可（その前提での改善提案）

---

## Pass 1: 正確性・ライフサイクル

### Findings（重要度順）
（なし）

### Resolved
- ~~**Critical**: `CNI_SUBNET` 指定時に `ipam.Subnet` が更新されず、`rangeStart/End` だけが上書きされるため、IPAM の整合性が崩れる可能性があります。~~ → `generator.go:73` で `ipam.Subnet = cidr.String()` に修正済み。
- ~~**High**: GC が名前空間内の全コンテナを無条件に削除します。~~ → `gc.go:71-76` で `LabelCreatedBy` + `LabelEsbEnv` によるフィルタリング実装済み。
- ~~**High**: GC で CNI の `Remove` が呼ばれず、IPAM 予約やiptablesが残留。~~ → `gc.go:40` で `r.removeCNI()` を呼び出し済み。
- ~~**Medium**: Docker ルートで IP が確定しない場合でも空の `IPAddress` を返すため、呼び出し側は失敗原因が不明になります。~~ → `docker/runtime.go:127-169` で指数バックオフ付きリトライ（5回）を実装。IP 未取得時は明示的エラーを返却。

### Open questions / Assumptions
- 名前空間 `meta.RuntimeNamespace` は ESB 専用という前提で設計されていますか？共用の場合は GC の削除対象を必ず限定する必要があります。

### Change summary（提案）
- CNI 生成時に `ipam.Subnet` を `CNI_SUBNET` と一致させる。
- GC 対象を `LabelCreatedBy` や `esb-{env}-` プレフィックスに限定し、CNI `Remove` を明示的に呼ぶ。
- Docker の IP 取得は非同期性を考慮したリトライ/待機、またはエラー返却に変更。

---

## Pass 2: セキュリティ・アクセス境界

### Resolved
- ~~**High**: gRPC サーバは認証/暗号化なしで起動し、reflection も常時有効です。~~ → gRPC TLS をデフォルトで有効化（`AGENT_GRPC_TLS_DISABLED=1` で無効化可能）し、メッセージ出力を `slog` で統一。
- ~~**High**: `InvokeWorker` が呼び出し側指定の `ip_address`/`port` をそのまま信頼。SSRF/Pivot のリスク。~~ → `server.go:60-70` で `container_id` ベースに変更、`workerCache` からIP解決するため外部指定不可。
- ~~**Medium**: `InvokeWorker` が `io.ReadAll` でレスポンスを無制限に読み込むため、巨大レスポンスでメモリ枯渇を招きます。~~ → `server.go:125-132` で `io.LimitReader` + `AGENT_INVOKE_MAX_RESPONSE_SIZE` 環境変数により上限設定可能に。

---

## Pass 3: 互換性・実行時エラー耐性

### Findings（重要度順）
- **Medium**: CA が存在する場合、全レジストリで HTTPS を強制します。開発/エッジ環境の insecure registry では失敗し、回避策が未整備です。`services/agent/internal/runtime/containerd/image.go:101`
  - Status: Resolved（HTTPS 強制の方針を明文化、insecure はサポート外と決定）
- **Low**: コンテナ名がユーザ入力の `FunctionName` と短いIDで構成されるため、名前制約や衝突の可能性があります。`services/agent/internal/runtime/containerd/runtime.go:234` / `services/agent/internal/runtime/docker/runtime.go:65`

### Resolved
- ~~**High**: `GetImage` のエラー種別を判別せず「未取得」として Pull に進む。~~ → `image.go:36` で `errdefs.IsNotFound` を確認し、他エラーは即失敗するよう修正済み。
- ~~**Medium**: Docker の Pause/Resume が未実装で、API 側は `Internal` 扱いになります。~~ → `docker/runtime.go:157-164` で `codes.Unimplemented` を返すよう修正済み。

---

## Pass 4: 運用性・可観測性

### Resolved
- ~~**Medium**: ログが `log.Printf` と `fmt.Printf` 混在、さらに Spec の詳細が常時 DEBUG 出力されます。~~ → `log/slog` による統一ロギングパッケージを導入。`main.go` を移行済み。
- ~~**Medium**: `InvokeWorker` の成功/失敗やレイテンシ指標が取得できず、性能劣化やコールドスタートの調査が困難です。~~ → `go-grpc-middleware/v2` のロギングインターセプターを導入。全 gRPC メソッドでレイテンシと結果を構造化ログとして出力可能。
- ~~**Low**: gRPC Health サービスが未導入で、起動判定は TCP のみになります。~~ → `main.go:155-160` で標準の `grpc/health` サービスを登録済み。
- ~~**Low**: `LastUsedAt` が `Ensure/Resume` のみ更新され、将来の再利用設計に対して誤った GC 指標になります。~~ → `ContainerRuntime.Touch()` を導入し、`InvokeWorker` 開始時に呼び出すことで正確な利用時間を記録。Docker 側にも `accessTracker` を実装済み。

---

## Pass 5: 保守性・一貫性

### Findings（重要度順）
- **Medium**: 設定値のデフォルトが `main.go` と `server.go` に散在し、テストや変更が難しくなっています。`services/agent/cmd/agent/main.go:30` / `services/agent/internal/api/server.go:68`
- **Low**: `Metrics` が Docker で未実装のままインタフェースに含まれており、利用側が runtime 特性を知る必要があります。`services/agent/internal/runtime/interface.go:43` / `services/agent/internal/runtime/docker/runtime.go:225`

### Resolved
- ~~**Low**: `PortAllocator` は実利用箇所がなく、未使用コードとして保守コストを増やしています。~~ → `port_allocator.go` および `port_allocator_test.go` を削除済み。
- ~~**Medium**: `ContainerState.Status` の意味が runtime により異なります。~~ → `runtime/interface.go` に共通定数（`RUNNING`, `PAUSED` 等）を定義し、両 runtime で正規化して返却するように修正。

### Open questions / Assumptions
- 上位コンポーネントは status 値をどの程度解釈していますか？

### Change summary（提案）
- ステータスは共通 enum（例: `RUNNING/PAUSED/STOPPED/UNKNOWN`）で正規化。
- 設定を `config` パッケージに集約し、デフォルトと環境変数を一元管理。
- 未使用コードは削除するか、使うなら設計意図を明確化。
- 機能差を `Capabilities` などで表現し、API から明示できるようにする。
