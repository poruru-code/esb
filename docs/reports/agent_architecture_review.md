# Agent 実装アーキテクチャレビュー（5 Passes）

対象: `services/agent`
制約: コンテナ分離は不可（その前提での改善提案）

---

## Pass 1: 正確性・ライフサイクル

### Findings（重要度順）
- **Medium**: Docker ルートで IP が確定しない場合でも空の `IPAddress` を返すため、呼び出し側は失敗原因が不明になります。再試行や待機、もしくはエラー返却が必要です。`services/agent/internal/runtime/docker/runtime.go:130-144`

### Resolved
- ~~**Critical**: `CNI_SUBNET` 指定時に `ipam.Subnet` が更新されず、`rangeStart/End` だけが上書きされるため、IPAM の整合性が崩れる可能性があります。~~ → `generator.go:73` で `ipam.Subnet = cidr.String()` に修正済み。
- ~~**High**: GC が名前空間内の全コンテナを無条件に削除します。~~ → `gc.go:71-76` で `LabelCreatedBy` + `LabelEsbEnv` によるフィルタリング実装済み。
- ~~**High**: GC で CNI の `Remove` が呼ばれず、IPAM 予約やiptablesが残留。~~ → `gc.go:40` で `r.removeCNI()` を呼び出し済み。

### Open questions / Assumptions
- 名前空間 `meta.RuntimeNamespace` は ESB 専用という前提で設計されていますか？共用の場合は GC の削除対象を必ず限定する必要があります。

### Change summary（提案）
- CNI 生成時に `ipam.Subnet` を `CNI_SUBNET` と一致させる。
- GC 対象を `LabelCreatedBy` や `esb-{env}-` プレフィックスに限定し、CNI `Remove` を明示的に呼ぶ。
- Docker の IP 取得は非同期性を考慮したリトライ/待機、またはエラー返却に変更。

---

## Pass 2: セキュリティ・アクセス境界

### Findings（重要度順）
- **High**: gRPC サーバは認証/暗号化なしで起動し、reflection も常時有効です。外部公開時に API 探索と操作が容易になります。`services/agent/cmd/agent/main.go:140`
  - Status: Partial（reflection は環境変数で制御、mTLS は任意 + 起動時警告）
- **Medium**: Docker 実装でリソース制限が未設定、containerd でも CPU 制限が無いため、1関数がホストを占有できます（分離不能な前提でも可用性リスク）。`services/agent/internal/runtime/docker/runtime.go:104`

### Resolved
- ~~**High**: `InvokeWorker` が呼び出し側指定の `ip_address`/`port` をそのまま信頼。SSRF/Pivot のリスク。~~ → `server.go:60-70` で `container_id` ベースに変更、`workerCache` からIP解決するため外部指定不可。
- ~~**Medium**: `InvokeWorker` が `io.ReadAll` でレスポンスを無制限に読み込むため、巨大レスポンスでメモリ枯渇を招きます。~~ → `server.go:125-132` で `io.LimitReader` + `AGENT_INVOKE_MAX_RESPONSE_SIZE` 環境変数により上限設定可能に。

### Open questions / Assumptions
- Agent の gRPC ポートはクラスタ内部のみで到達可能という前提ですか？

### Change summary（提案）
- mTLS/トークン認証、reflection の環境別制御、リクエストサイズ制限の導入。
- `InvokeWorker` を `container_id` ベースに変更し、IP をサーバ側解決にする。
- CPU/メモリ上限の導入（分離ではなく資源制御として実装可能）。

---

## Pass 3: 互換性・実行時エラー耐性

### Findings（重要度順）
- **Medium**: CA が存在する場合、全レジストリで HTTPS を強制します。開発/エッジ環境の insecure registry では失敗し、回避策が未整備です。`services/agent/internal/runtime/containerd/image.go:101`
- **Low**: コンテナ名がユーザ入力の `FunctionName` と短いIDで構成されるため、名前制約や衝突の可能性があります。`services/agent/internal/runtime/containerd/runtime.go:234` / `services/agent/internal/runtime/docker/runtime.go:65`

### Resolved
- ~~**High**: `GetImage` のエラー種別を判別せず「未取得」として Pull に進む。~~ → `image.go:36` で `errdefs.IsNotFound` を確認し、他エラーは即失敗するよう修正済み。
- ~~**Medium**: Docker の Pause/Resume が未実装で、API 側は `Internal` 扱いになります。~~ → `docker/runtime.go:157-164` で `codes.Unimplemented` を返すよう修正済み。

### Open questions / Assumptions
- containerd 利用時のレジストリは TLS を前提に設計されていますか？insecure registry のニーズはありますか？

### Change summary（提案）
- `GetImage` のエラーは `NotFound` のみ Pull 対象にし、それ以外は即失敗。
- レジストリの HTTPS/HTTP 方針を環境変数で制御可能にする。
- Pause/Resume 未実装時は `codes.Unimplemented` を返し、UI/CLI 側で分岐できるようにする。
- コンテナ名はハッシュ化/サニタイズし、表示名はラベルに保持する。

---

## Pass 4: 運用性・可観測性

### Findings（重要度順）
- **Medium**: ログが `log.Printf` と `fmt.Printf` 混在、さらに Spec の詳細が常時 DEBUG 出力されます。ログ制御と機微情報の取り扱いが不明確です。`services/agent/internal/runtime/containerd/runtime.go:288` / `services/agent/internal/runtime/docker/runtime.go:73`
- **Medium**: `InvokeWorker` の成功/失敗やレイテンシ指標が取得できず、性能劣化やコールドスタートの調査が困難です。`services/agent/internal/api/server.go:50`
- **Low**: gRPC Health サービスが未導入で、起動判定は TCP のみになります。`services/agent/cmd/agent/main.go:140`
- **Low**: `LastUsedAt` が `Ensure/Resume` のみ更新され、将来の再利用設計に対して誤った GC 指標になります。`services/agent/internal/runtime/containerd/runtime.go:345`

### Open questions / Assumptions
- ログ出力はホスト側で収集される設計ですか？それともコンテナ内で完結させますか？

### Change summary（提案）
- ログ出力を統一（構造化 or 一貫した logger）し、DEBUG は環境変数で制御。
- Invoke の計測（開始/終了、リトライ回数、ステータス）を追加。
- gRPC Health を導入して readiness/liveness を明確化。

---

## Pass 5: 保守性・一貫性

### Findings（重要度順）
- **Medium**: `ContainerState.Status` の意味が runtime により異なります（containerd は `RUNNING`、docker は `running` など）。上位のGC/監視が誤解する可能性があります。`services/agent/internal/runtime/interface.go:23` / `services/agent/internal/runtime/containerd/runtime.go:131` / `services/agent/internal/runtime/docker/runtime.go:213`
- **Medium**: 設定値のデフォルトが `main.go` と `server.go` に散在し、テストや変更が難しくなっています。`services/agent/cmd/agent/main.go:30` / `services/agent/internal/api/server.go:68`
- **Low**: `Metrics` が Docker で未実装のままインタフェースに含まれており、利用側が runtime 特性を知る必要があります。`services/agent/internal/runtime/interface.go:43` / `services/agent/internal/runtime/docker/runtime.go:225`

### Resolved
- ~~**Low**: `PortAllocator` は実利用箇所がなく、未使用コードとして保守コストを増やしています。~~ → `port_allocator.go` および `port_allocator_test.go` を削除済み。

### Open questions / Assumptions
- 上位コンポーネントは status 値をどの程度解釈していますか？

### Change summary（提案）
- ステータスは共通 enum（例: `RUNNING/PAUSED/STOPPED/UNKNOWN`）で正規化。
- 設定を `config` パッケージに集約し、デフォルトと環境変数を一元管理。
- 未使用コードは削除するか、使うなら設計意図を明確化。
- 機能差を `Capabilities` などで表現し、API から明示できるようにする。
