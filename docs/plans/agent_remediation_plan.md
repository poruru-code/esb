# Agent 対応計画（High/Critical）

目的: `docs/reports/agent_architecture_review.md` の High/Critical を優先度順に整理し、対応の段取りを明確化する。
前提: コンテナ分離は不可。資源制御と境界強化でリスクを低減する。

## 優先度の基準
- P0: ネットワーク破綻やデータ損失など、即時の可用性リスク
- P1: 外部到達時のセキュリティ境界、または環境全体に波及する運用リスク
- P2: リリース後に障害原因の特定を困難にする実行時エラー耐性

## 対応項目（優先度順）

### P0-1: CNI_SUBNET と IPAM の整合性修正（Critical）
- Rationale: `CNI_SUBNET` 指定時に `ipam.Subnet` が更新されず、IPAM の整合性が崩れる可能性があるため即時修正が必要。
- Scope: `services/agent/internal/cni/generator.go`
- Proposed change: `subnet` 指定時は `ipam.Subnet` をその CIDR に更新し、`rangeStart/End` を同一 CIDR 内に限定する。
- Acceptance: 生成された `.conflist` の `subnet` と `rangeStart/End` が一致し、起動後に IP 割当が正常に行われる。
- Tests/Docs: CNI 生成のユニットテスト追加、`docs/environment-variables.md` への `CNI_SUBNET` 説明補足。

### P1-1: InvokeWorker の SSRF/ピボット対策（High）
- Rationale: `ip_address`/`port` をクライアント任せにする設計は内部ネットワークへの SSRF を許容する。
- Scope: `services/agent/internal/api/server.go`、`proto/`（API変更が必要な場合）
- Proposed change: `container_id` を入力にし、IP 解決は runtime で実施する。互換維持が必要なら `ip_address` を許容しつつ CNI 範囲/port=8080 に制限。
- Acceptance: 任意の IP に対する呼び出しが拒否され、正規コンテナのみが Invoke 可能。
- Tests/Docs: 既存クライアントとの互換性方針を明記し、拒否ケースのテストを追加。

### P1-2: gRPC の境界保護（High）
- Rationale: 認証/暗号化なし + reflection 常時有効は外部到達時の API 探索/操作を容易にする。
- Scope: `services/agent/cmd/agent/main.go`
- Proposed change: mTLS またはトークン認証を導入し、reflection は `AGENT_GRPC_REFLECTION=1` の時のみ有効化。
- Acceptance: 認証なしの接続が拒否され、reflection がデフォルトで無効。
- Tests/Docs: gRPC 接続テスト追加、認証/証明書の設定手順をドキュメント化。
- Status: Done（mTLS 有効時のクライアント証明書必須化 + reflection のデフォルト無効化）

### P1-3: GC の対象限定と CNI Cleanup（High）
- Rationale: GC が名前空間内全削除 + CNI Remove 未実行は誤削除/残留状態を引き起こす。
- Scope: `services/agent/internal/runtime/containerd/gc.go`、`services/agent/internal/runtime/containerd/runtime.go`
- Proposed change: `LabelCreatedBy`/`esb-{env}-` で対象を限定し、タスクが存在する場合は `cni.Remove` を必ず実行する。
- Acceptance: GC 実行後に対象外コンテナは残り、CNI 残留が発生しない。
- Tests/Docs: GC 対象フィルタのユニットテスト、運用手順の更新。

### P2-1: Image 取得エラーの判別（High）
- Rationale: `GetImage` のエラー種別を判別せず Pull するため、権限/通信/namespace の原因が隠蔽される。
- Scope: `services/agent/internal/runtime/containerd/image.go`
- Proposed change: `errdefs.IsNotFound(err)` のみ Pull し、その他は即失敗にする。
- Acceptance: 誤った Pull に進まず、原因がログ/エラーで明確に判別できる。
- Tests/Docs: `GetImage` エラー種別ごとのテスト追加。

## 依存関係と順序
- P0-1 は単独で即時対応可能。
- P1-1 は API 変更を伴う場合があるため、互換モード（CNI 範囲/port 制限）→ `container_id` 移行の順で段階導入する。
- P1-2 は P1-1 と同一リリースが望ましいが、認証導入が重い場合は reflection 無効化を先行してもよい。
- P1-3 は P1-1/P1-2 と独立だが、運用リスク低減のため早期に実施。
- P2-1 は P1 系と独立。

## リスクとロールバック
- gRPC 認証導入と API 変更はクライアント影響が大きいので、段階導入と互換期間を設ける。
- CNI/GC 周辺の変更は環境差分の影響を受けるため、段階的にステージングで検証する。
