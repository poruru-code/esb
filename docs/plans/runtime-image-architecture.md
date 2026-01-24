<!--
Where: docs/plans/runtime-image-architecture.md
What: Runtime別イメージのアーキテクチャ設計書。
Why: 実装者がこの1文書だけで作業できる設計仕様を提供する。
-->

# Runtime別イメージ アーキテクチャ設計書

ステータス: 提案
作成日: 2026-01-24
オーナー: Architecture

## 1. 目的
- ランタイム差分を明確に分離し、運用時の曖昧さを排除する。
- 不変タグによる再現性と説明可能性を担保する。
- 依存関係を最小化し、責務境界を明確化する。
- 本設計書のみで実装可能な詳細仕様を提供する。

## 2. 非目的
- 本書は実装を行わない。
- 後方互換は考慮しない。単一リリースで全面切替を行う。

## 3. 用語
- コンポーネント: agent / gateway / runtime-node / provisioner
- ランタイム: docker / containerd / firecracker
- ランタイム系統: docker / containerd（containerd + firecracker を包含）
- 変種: コンポーネント × ランタイム系統の組み合わせ
- 不変タグ: 公開後に内容が変わらないタグ

## 4. アーキテクチャ原則
1) ランタイム差分は「製品差分」であり、同一イメージで吸収しない。
2) イメージ名とラベルで責務・ランタイムが即判別できること。
3) 本番は不変タグのみを使用する。
4) 依存関係は責務に一致させ、不要な同梱を禁止する。
5) ランタイム不一致は起動時に必ず失敗させる。

## 5. イメージ体系
### 5.1 命名規則
- 形式: `<registry>/<brand>-<component>-<runtime>`
- runtime: `docker`, `containerd`（containerd と firecracker を包含する系統名として使用）
- component: `agent`, `gateway`, `runtime-node`, `provisioner`

### 5.2 変種マトリクス
- agent: docker / containerd
- gateway: docker / containerd
- runtime-node: containerd のみ（docker モードでは使用しない）
- provisioner: docker / containerd

### 5.3 例
- `registry.example.com/<brand>-agent-containerd`
- `registry.example.com/<brand>-gateway-containerd`
- `registry.example.com/<brand>-runtime-node-containerd`

## 6. タグ戦略
### 6.1 許可タグ
- `vX.Y.Z`（本番必須）
- `vX.Y`（互換範囲）
- `vX`（長期互換）
- `vX.Y.Z-rc.N`（プレリリース）
- `sha-<git-short>`（CI検証）
- `latest`（開発用途のみ・本番禁止）

### 6.2 ポリシー
- 本番は不変タグのみ使用。
- 全ランタイムで同一バージョンを同時公開する。
- `latest` は開発用途限定とし、本番利用を禁止する。

## 7. ベースイメージ方針
- `os-base` / `python-base` はランタイム非依存とする。
- ランタイム固有の依存は変種 Dockerfile にのみ配置する。

## 7.5 関数イメージ方針（SAM 生成物）
- SAM テンプレートから生成される関数イメージは **ランタイム非依存の共通成果物**とする。
- ランタイム差分は制御面（agent / runtime-node / gateway）が吸収し、関数イメージには持ち込まない。
- 例外: Firecracker 固有の制約によりベースイメージや依存が変わる必要がある場合のみ、関数イメージの分岐を許可する。

## 8. コンポーネント要件
### 8.1 agent
#### agent-docker
- CNI プラグインや containerd ツールを同梱しない。
- CNI 設定生成を行わない。
- ランタイムガード: `IMAGE_RUNTIME=docker`。

#### agent-containerd
- CNI プラグイン（bridge/host-local/loopback/portmap）を同梱する。
- `iptables`, `iproute2` を必須とする。
- WireGuard は同梱してよい（containerd / firecracker 両方で利用可能な前提）。
- ランタイムガード: `IMAGE_RUNTIME=containerd`。
- `CONTAINERD_RUNTIME=aws.firecracker` は起動時設定で切替（デフォルトは containerd）。

### 8.2 gateway
#### gateway-docker
- WireGuard ツールを同梱しない。
- ランタイムガード: `IMAGE_RUNTIME=docker`。

#### gateway-containerd
- WireGuard ツール（`wireguard-tools`, `wireguard-go`）を同梱する。
- ルート適用ヘルパーを含む。
- WireGuard は明示的に有効化された場合のみ起動すること。
- ランタイムガード: `IMAGE_RUNTIME=containerd`。

### 8.3 runtime-node
#### runtime-node-containerd
- containerd + CNI を必須とする。
- WireGuard を同梱し、必要時のみ起動する。
- ランタイムガード: `IMAGE_RUNTIME=containerd`。

### 8.4 provisioner
- docker / containerd の2分割とする。
- runtime 不一致を検出し失敗させる。

## 9. Dockerfile 構成
- ランタイム変種ごとに Dockerfile を分割する。
- 共通ビルドは `Dockerfile.builder` に集約する。

### 9.1 ファイル配置例
```
services/agent/Dockerfile.builder
services/agent/Dockerfile.docker
services/agent/Dockerfile.containerd
```

### 9.2 必須 Build Args
- `<BRAND>_VERSION`
- `GIT_SHA`
- `BUILD_DATE`
- `IMAGE_RUNTIME`
- `COMPONENT`

### 9.3 必須 ENV（イメージに焼き込み）
- `<BRAND>_VERSION`
- `IMAGE_RUNTIME`
- `COMPONENT`

## 10. Runtime Guard（Fail-Fast）
- 起動時に runtime 不一致を必ず検出し終了する。
- 例: `AGENT_RUNTIME` と `IMAGE_RUNTIME` が不一致なら即エラー。
- `containerd` は containerd / firecracker の両方を許容する。
- エラーメッセージは具体的かつ運用者が対応可能な内容とする。

## 10.5 WireGuard 有効化条件と起動フロー
### 10.5.1 共通ルール
- WireGuard は **明示条件を満たした場合のみ**起動する。
- 追加の環境変数は導入せず、既存条件で判定する。
- gateway: `WG_CONF_PATH` が存在し、`/dev/net/tun` が利用可能な場合のみ起動する。
- runtime-node: `WG_CONTROL_NET` が指定された場合のみルートを設定する。
- WireGuard 失敗時は **警告のみ**で継続する（厳格化は監視/ヘルスチェック側で担保）。

### 10.5.2 gateway（containerd）起動フロー
1) `WG_CONF_PATH` の存在と `/dev/net/tun` の存在を確認  
2) 条件を満たす場合のみ WireGuard 起動  
3) `wireguard-go` が存在すれば userspace 実装を強制  
4) `wg-quick up <WG_CONF_PATH>` を実行  
5) 接続後に route 補正（AllowedIPs 反映）を実行  
6) `GATEWAY_WORKER_ROUTE_*` が指定されていれば追加ルートを適用  
7) いずれかの手順が失敗した場合は警告して継続  

#### gateway 環境変数（内部管理のみ）
- `WG_CONF_PATH`: 既定 `/app/config/wireguard/wg0.conf`  
- `WG_INTERFACE`: 既定 `wg0`  
- `GATEWAY_WORKER_ROUTE_VIA_HOST`: ルートの next-hop 解決用ホスト名  
- `GATEWAY_WORKER_ROUTE_VIA`: ルートの next-hop IP（直接指定）  
- `GATEWAY_WORKER_ROUTE_CIDR`: ルート対象 CIDR（既定 `10.88.0.0/16`）  

### 10.5.3 runtime-node（containerd）起動フロー
補足: runtime-node コンテナは WireGuard の起動（wg-quick）は行わず、**制御ネットへのルーティングのみ**を担当する。
1) `WG_CONTROL_NET` が空ならルート設定をスキップ  
2) `WG_CONTROL_GW` / `WG_CONTROL_GW_HOST` / デフォルトゲートウェイの順で next-hop を解決  
3) `WG_CONTROL_NET` 宛のルートを追加  
4) `WG_CONTROL_GW_HOST` が指定されている場合は watcher で再解決を行う  
5) 解決に失敗した場合は警告して継続  

#### runtime-node 環境変数（内部管理のみ）
- `WG_CONTROL_NET`: ルート対象 CIDR（例: `10.99.0.0/24`）  
- `WG_CONTROL_GW`: ルート next-hop IP（直接指定）  
- `WG_CONTROL_GW_HOST`: ルート next-hop のホスト名  

## 11. OCI ラベル（必須）
- `org.opencontainers.image.title`
- `org.opencontainers.image.version`
- `org.opencontainers.image.revision`
- `org.opencontainers.image.source`
- `org.opencontainers.image.created`
- `org.opencontainers.image.vendor`
- `com.<brand>.component`
- `com.<brand>.runtime`
- `com.<brand>.version`
※ `<brand>` は branding で生成される `meta` の値（例: acme）を使用し、ハードコードしない。

## 12. Compose / CLI 仕様
### 12.1 共通環境変数（内部/外部を含む）
- `<BRAND>_REGISTRY`
- `<BRAND>_TAG`
- `<BRAND>_VERSION`

### 12.4 環境変数の最小化と分類
#### 外部指定（運用者が必要時のみ設定）
- `<BRAND>_REGISTRY`: 取得先レジストリを切替える場合のみ。
- `<BRAND>_TAG`: 参照する不変タグ（本番は必須）。
※ 外部指定は原則この2つのみとし、追加は設計変更として扱う。

#### 内部管理（実装またはCLI/Composeが設定）
- `<BRAND>_VERSION`: ビルド時に埋め込む。
- `IMAGE_RUNTIME`: イメージに焼き込む（ブランド非依存）。
- `COMPONENT`: イメージに焼き込む（ブランド非依存）。
- `AGENT_RUNTIME`: CLI/Compose が設定（運用者が変更しない）。
- `CONTAINERD_RUNTIME`: firecracker を選択する場合に CLI/Compose が設定。
- `WG_QUICK_USERSPACE_IMPLEMENTATION`: gateway の起動中に内部で設定。
- `WG_QUICK_USERSPACE_IMPLEMENTATION_FORCE`: gateway の起動中に内部で設定。
- `WG_CONF_PATH`: gateway の WireGuard 設定パス（既定値を使用）。
- `WG_INTERFACE`: gateway の WireGuard インターフェース名（既定値を使用）。
- `GATEWAY_WORKER_ROUTE_VIA_HOST`: gateway のルート解決（必要時のみ内部設定）。
- `GATEWAY_WORKER_ROUTE_VIA`: gateway のルート解決（必要時のみ内部設定）。
- `GATEWAY_WORKER_ROUTE_CIDR`: gateway のルート対象（必要時のみ内部設定）。
- `WG_CONTROL_NET`: runtime-node の制御ネット CIDR（必要時のみ内部設定）。
- `WG_CONTROL_GW`: runtime-node の next-hop（必要時のみ内部設定）。
- `WG_CONTROL_GW_HOST`: runtime-node の next-hop（必要時のみ内部設定）。

### 12.5 ブランド反映ルール
- `<BRAND>_` は branding 生成の `meta.EnvPrefix` を使用する。
- 例: brand が `acme` の場合 `ACME_REGISTRY` / `ACME_TAG` / `ACME_VERSION` を使用する。
- 固定プレフィクスの外部変数は使用しない（後方互換は設計範囲外）。

## 18. 詳細設計（実装仕様）
### 18.1 画像名・タグ（確定仕様）
- 画像名は `<brand>-<component>-<runtime>` に固定する。
- `<runtime>` は `docker` / `containerd` の2系統のみ。
- `containerd` 画像は firecracker を包含する（`CONTAINERD_RUNTIME=aws.firecracker` で切替）。
- タグは `vX.Y.Z` を正とし、`latest` は開発用途のみで許容する。

### 18.2 Build Args / ENV / ラベル
- Build Args（固定）: `<BRAND>_VERSION`, `GIT_SHA`, `BUILD_DATE`, `IMAGE_RUNTIME`, `COMPONENT`
- ENV（イメージに焼き込み）: `<BRAND>_VERSION`, `IMAGE_RUNTIME`, `COMPONENT`
- OCI ラベル: `org.opencontainers.*` + `com.<brand>.*` を必須とする。

### 18.3 Runtime Guard 実装位置（確定仕様）
- guard は **entrypoint** で実施する（最優先）。
- 追加の環境変数は導入しない。
- 判定に使う環境変数は既存の `IMAGE_RUNTIME`, `AGENT_RUNTIME`, `CONTAINERD_RUNTIME` のみ。

#### 18.3.1 agent の guard（必須）
- `IMAGE_RUNTIME=docker` の場合: `AGENT_RUNTIME` が `docker` 以外なら即終了。
- `IMAGE_RUNTIME=containerd` の場合: `AGENT_RUNTIME` が `containerd` 以外なら即終了。
- `AGENT_RUNTIME` が未設定の場合は **即終了**（曖昧さを許さない）。

#### 18.3.2 runtime-node の guard（必須）
- `IMAGE_RUNTIME=containerd` でのみ起動する。
- `CONTAINERD_RUNTIME=aws.firecracker` の場合は firecracker ルートを使用する。
- それ以外は containerd ルートを使用する。

#### 18.3.3 gateway / provisioner の guard（必須）
- `IMAGE_RUNTIME` が `docker` または `containerd` 以外なら即終了。
- runtime 判定のための新しい外部変数は導入しない。
- コンテナは **選択されたイメージ名が正**であることを前提とする。

### 18.4 Compose / CLI の運用規約
- 外部入力は `<BRAND>_REGISTRY` と `<BRAND>_TAG` のみ。
- `latest` を指定できるのは開発用途のみ。運用用途では禁止。
- `AGENT_RUNTIME` と `CONTAINERD_RUNTIME` は CLI/Compose で明示設定する。

### 18.5 containerd / firecracker 切替（確定仕様）
- 切替キーは `CONTAINERD_RUNTIME=aws.firecracker`。
- 切替は runtime-node / agent の起動時判定にのみ利用する。

### 18.6 WireGuard（最小環境変数での運用）
- 追加の外部変数は導入しない。
- gateway: `WG_CONF_PATH` が存在する場合のみ WireGuard 起動。
- runtime-node: `WG_CONTROL_NET` が指定された場合のみルート設定。

### 12.2 Compose 記述例
- `image: ${<BRAND>_REGISTRY}/<brand>-agent-containerd:${<BRAND>_TAG}`

### 12.3 CLI マッピング
- docker -> `<brand>-<component>-docker`
- containerd / firecracker -> `<brand>-<component>-containerd`

## 13. CI/CD ビルドマトリクス
- 次元: component × runtime系統 × arch
- arch: amd64 / arm64
- 出力:
  - 不変タグの全変種
  - SBOM
  - 署名付き provenance（推奨）

## 14. 構造テスト（必須）
- agent-docker: CNI が存在しないこと
- agent-containerd: CNI が存在すること
- gateway-containerd: WireGuard バイナリが存在すること
- runtime-node-containerd: WireGuard バイナリが存在すること

## 15. 切替方針（後方互換なし）
- 旧イメージ名・旧タグはすべて廃止。
- 新命名規則に一括切替。
- リリースノートで新命名規則のみを提示。
- 全環境で不変タグのみを使用する。

## 16. 受け入れ基準
- 全ランタイムで別イメージが存在する。
- runtime 不一致は起動時に必ず失敗する。
- 本番は不変タグのみで運用可能。
- すべてのイメージに必須 OCI ラベルが付与されている。
- 構造テストが全変種で通過する。

## 17. リスクと対策
- リスク: 一括切替の混乱
  - 対策: リリースノートの明確化と runtime guard の強制
- リスク: ビルドマトリクスの増大
  - 対策: 自動化されたマトリクスビルド
- リスク: 依存差分の逸脱
  - 対策: 構造テストと依存リストの明文化
