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

### 12.2 Compose 記述例
- `image: ${<BRAND>_REGISTRY}/<brand>-agent-containerd:${<BRAND>_TAG}`

### 12.3 CLI マッピング
- docker -> `<brand>-<component>-docker`
- containerd / firecracker -> `<brand>-<component>-containerd`

### 12.4 環境変数の最小化と分類
#### 外部指定（運用者が必要時のみ設定）
- `<BRAND>_REGISTRY`: 取得先レジストリを切替える場合のみ。
- `<BRAND>_TAG`: 参照する不変タグ（本番は必須）。
※ 外部指定は原則この2つのみとし、追加は設計変更として扱う。

#### 内部管理（実装またはCLI/Composeが設定）
- `<BRAND>_VERSION`: ビルド時に埋め込む。
- `IMAGE_RUNTIME`: イメージに焼き込む。
- `COMPONENT`: イメージに焼き込む。
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
- リスク: branding 生成に失敗し ENV_PREFIX が設定されない
  - 対策: applyRuntimeEnv の先頭で ENV_PREFIX を検証し即失敗

## 18. 実装計画（フェーズ分割）
### 18.1 Phase 0: 影響範囲の棚卸し
- 対象ファイルと環境変数の洗い出し（IMAGE_* / 旧プレフィクスの残存確認）。
- branding 生成値（meta.EnvPrefix / meta.ImagePrefix / meta.LabelPrefix）の利用箇所を整理。
受け入れ条件:
- 影響範囲一覧が完成し、変更対象が確定している。
- branding 生成が失敗した場合の停止条件が合意されている。

### 18.2 Phase 1: 画像命名・タグの統一
- 画像名を `<brand>-<component>-{docker|containerd}` に統一。
- `latest` は開発用途のみ許容、運用は `vX.Y.Z` のみ。
受け入れ条件:
- 画像名の命名規則が実装全体で一致している。
- 開発以外で `latest` を使う経路がない。

### 18.3 Phase 2: 外部入力の最小化
- 外部入力を `<BRAND>_REGISTRY` / `<BRAND>_TAG` のみに統一。
- `IMAGE_PREFIX` / `IMAGE_TAG` / `FUNCTION_IMAGE_PREFIX` の外部利用を廃止。
受け入れ条件:
- 生成物に `${IMAGE_TAG}` 等のプレースホルダが残っていない。
- Compose と CLI に外部入力が2つだけになっている。

### 18.4 Phase 3: Dockerfile とビルド引数の整理
- Dockerfile の `ARG IMAGE_PREFIX=<brand>` など固定デフォルトを撤去。
- `IMAGE_RUNTIME` / `COMPONENT` / `<BRAND>_VERSION` を ENV に焼き込む。
- `<BRAND>_VERSION` の注入経路（CLI/CI）を確定する。
受け入れ条件:
- すべてのサービスイメージに `IMAGE_RUNTIME` と `COMPONENT` が入っている。
- ブランド固定のデフォルト値が残っていない。
- `<BRAND>_VERSION` が未設定の場合にビルドが失敗する。

### 18.5 Phase 4: Runtime Guard 実装
- entrypoint に `IMAGE_RUNTIME` と `AGENT_RUNTIME` の整合チェックを追加。
- 不一致時は明確なエラーで即終了。
受け入れ条件:
- 不一致条件で必ず起動失敗する。
- 一致条件では従来通りに起動する。

### 18.6 Phase 5: containerd / firecracker 切替の統一
- `CONTAINERD_RUNTIME=aws.firecracker` のみで切替できることを保証。
- firecracker モードでは containerd 画像を流用し、**entrypoint ラッパー**で切替する。
受け入れ条件:
- containerd / firecracker どちらでも同一イメージが使える。

### 18.7 Phase 6: E2E 更新
- E2E で新しい命名規則と外部入力のみを使用。
- firecracker 相当は `CONTAINERD_RUNTIME=aws.firecracker` で再現。
受け入れ条件:
- すべての E2E プロファイルが成功する。

### 18.8 Phase 7: 運用ルールと移行ガイドの整備
- 生成物再作成（`functions.yml` の image 完全埋め込み）の運用ルールを明文化。
- 旧環境変数（`IMAGE_TAG` など）の廃止をリリースノートに明記。
受け入れ条件:
- “タグ変更時は再生成が必須” が運用ドキュメントに記載されている。
- 旧変数を使った運用が禁止されている。

## 19. 詳細設計（コードレベル）
### 19.1 環境変数の解決方法
- 外部入力は `<BRAND>_REGISTRY` / `<BRAND>_TAG` のみ。
- `<BRAND>` は `meta.EnvPrefix` から動的に生成する。
- `envutil.HostEnvKey` は `ENV_PREFIX` を前提にし、固定デフォルトは使わない。
- `ENV_PREFIX` は **必ず `applyBrandingEnv` により先に設定される**ことを保証する。
- `ENV_PREFIX` が未設定の場合は **即エラー**とし、暗黙のデフォルトは持たない。
- `applyRuntimeEnv` 冒頭で `ENV_PREFIX` を検証する専用チェックを追加する。

### 19.2 CLI の環境反映
対象:
- `cli/internal/helpers/env_defaults.go`
- `cli/internal/envutil/envutil.go`
 - `cli/internal/workflows/build.go`
 - `cli/internal/generator/go_builder.go`

設計:
- `applyRuntimeEnv` の先頭で `applyBrandingEnv` を実行し、`ENV_PREFIX` を必ず先に設定する。
- `IMAGE_TAG` / `IMAGE_PREFIX` を設定する処理を削除する。
- `<BRAND>_TAG` が未設定の場合は `latest` を使用（開発用途のみ想定）。
- `<BRAND>_VERSION` は **CLI または CI が必ず供給**する。未設定はビルド失敗とする。
- BuildRequest に `<BRAND>_VERSION` を明示的に渡し、generator 側で必須チェックする。
 - `buildCommand.Run` の直後に `<BRAND>_VERSION` を検証し、未設定なら CLI で即失敗する。

#### 19.2.1 `<BRAND>_VERSION` 解決手順（CLI）
1) `applyBrandingEnv` により `ENV_PREFIX` を設定する。  
2) `versionKey := envutil.HostEnvKey("VERSION")` を生成する。  
3) `version := os.Getenv(versionKey)` を取得する。  
4) 空の場合は **即エラー**（例: `ERROR: <BRAND>_VERSION is required`）。  
5) `BuildRequest.Version` に格納し、generator/build に伝播する。  

#### 19.2.2 `<BRAND>_VERSION` 供給責務（CI/運用）
- CI は必ず `<BRAND>_VERSION` を設定してビルドする。  
- リリース: Git タグ `vX.Y.Z` を `<BRAND>_VERSION` に設定する。  
- 開発/検証: `0.0.0-dev.<shortsha>` など明示的な値を設定する。  
- 未設定でのビルドは禁止（ビルド失敗）。  

#### 19.2.3 BuildRequest のフィールド追加（明示仕様）
- `cli/internal/workflows/build.go` の `BuildRequest` に `Version string` を追加する。  
- `cli/internal/generator/build_request.go` の `BuildRequest` に `Version string` を追加する。  
- `cli/internal/commands/build.go` の `buildCommand.Run` で `Version` を設定する。  
- 伝播ルール: workflow の `BuildRequest.Version` を generator の `BuildRequest.Version` にコピーする。  
- generator 側で `Version` が空の場合は即エラー（`ERROR: <BRAND>_VERSION is required`）。  
 - `Version` は **必ず `<BRAND>_VERSION` 由来**であること（`<BRAND>_TAG` からは設定しない）。  
 - `Version` は `buildDockerImage` の build args に `<BRAND>_VERSION` として渡す。  

#### 19.2.6 既存関数の置換位置（明示仕様）
- `cli/internal/generator/go_builder_helpers.go` の以下を置換:  
  - `resolveImageTag` → `<BRAND>_TAG` 解決ロジックに置換  
  - `resolveRegistryConfig` → `<BRAND>_REGISTRY` 解決ロジックに置換  
- `cli/internal/generator/go_builder.go` の `resolveImageTag(request.Env)` 呼び出しを削除し、  
  `request.Version` をタグとして使用する。  
- `resolveRegistryConfig(mode)` は registry 設定の自動生成を廃止し、  
  `<BRAND>_REGISTRY` の値のみを使用する。  
 - `resolveImageTag` は `error` を返すため、`GoBuilder.Build` でエラー処理を追加する。  

#### 19.2.7 置換後の関数仕様（明示）
- `resolveImageTag(version string) (string, error)`  
  - `version` が空なら `ERROR: <BRAND>_VERSION is required` を返す。  
  - 返値は `version` をそのまま返す。  
- `resolveRegistryConfig() registryConfig`  
  - 外部入力 `<BRAND>_REGISTRY` を正規化した値のみを返す。  
  - `Internal` は空（内部レジストリの自動設定は廃止）。  

#### 19.2.8 関数/呼び出しの差分イメージ（コード例）
**変更前（概略）**
```
mode := strings.TrimSpace(request.Mode)
registry := resolveRegistryConfig(mode)
imageTag := resolveImageTag(request.Env)
```

**変更後（概略）**
```
registry := resolveRegistryConfig()
imageTag, err := resolveImageTag(request.Version)
if err != nil {
  return err
}
```

#### 19.2.4 GIT_SHA / BUILD_DATE の解決手順（内部管理）
- これらは外部入力ではなく **CLI が内部で決定**する。  
- `GIT_SHA`:
  - 環境変数 `GIT_SHA` があればそれを優先。  
  - 未設定なら `git rev-parse --short HEAD` を実行して取得。  
  - 取得失敗時は `unknown` とし、ビルドは継続。  
- `BUILD_DATE`:
  - 環境変数 `BUILD_DATE` があればそれを優先。  
  - 未設定なら `UTC` の ISO8601 で生成する（例: `2026-01-24T12:00:00Z`）。  
  - 生成は `applyRuntimeEnv` 内で一度だけ行い、以後は上書きしない。  

#### 19.2.5 `<BRAND>_TAG` / `<BRAND>_REGISTRY` 解決手順（CLI）
- `tagKey := envutil.HostEnvKey("TAG")` を生成し、`<BRAND>_TAG` を取得する。  
- 未設定の場合は **`latest`** を使用（開発用途のみ）。  
- `registryKey := envutil.HostEnvKey("REGISTRY")` を生成し、`<BRAND>_REGISTRY` を取得する。  
- `Registry` は以下の正規化を行う:  
  - 空の場合は空文字（レジストリ指定なし）。  
  - 末尾に `/` が無ければ付与する。  
- `resolveImageTag` / `resolveRegistryConfig` は上記ロジックに置き換える。  
- **優先順位:** `BuildRequest.Version`（必須） → `<BRAND>_TAG`（開発のみ）。  

### 19.3 関数イメージの埋め込み生成
対象:
- `cli/internal/generator/templates/functions.yml.tmpl`
- `cli/internal/generator/renderer.go`
- `cli/internal/generator/renderer_test.go`
- `cli/internal/generator/testdata/renderer/functions_simple.golden`

設計:
- `functions.yml` の `image` は **完全な文字列**で出力する。
- テンプレートは以下の形式に変更:
  - `image: "{{ .Registry }}{{ .ImagePrefix }}-{{ .ImageName }}:{{ .Tag }}"`
- `Registry` は末尾 `/` を含む形に正規化して渡す（空の場合は空文字）。
- `ImagePrefix` は `meta.ImagePrefix` を使用し、外部入力にしない。
- `functions.yml` は **タグ変更時に必ず再生成**する運用ルールとする。

### 19.4 サービスイメージの命名とビルド
対象:
- `cli/internal/generator/go_builder.go`
- `cli/internal/generator/go_builder_helpers.go`
- 各 `docker-compose.*.yml`

設計:
- サービスイメージ名は `<brand>-<component>-{docker|containerd}` に固定。
- Compose は `<BRAND>_REGISTRY` / `<BRAND>_TAG` だけ参照する。
- `IMAGE_TAG` / `FUNCTION_IMAGE_PREFIX` / `IMAGE_PREFIX` は Compose から削除する。

#### 19.4.1 Build Args 注入ルール
- `buildDockerImage` に渡す build args は以下に固定する:  
  - `<BRAND>_VERSION`, `GIT_SHA`, `BUILD_DATE`, `IMAGE_RUNTIME`, `COMPONENT`  
- `IMAGE_RUNTIME` / `COMPONENT` は **サービスごとに固定値**を渡す。  
  - 例: agent-containerd -> `IMAGE_RUNTIME=containerd`, `COMPONENT=agent`  
- `<BRAND>_VERSION` は `BuildRequest.Version` から取得する。  
- `GIT_SHA` / `BUILD_DATE` は `applyRuntimeEnv` で解決済みの値を使う。  
- すべてのサービスイメージに同一のラベルセットを付与する。  

#### 19.4.2 buildDockerImage の引数順序（固定）
- build args は **同一順序**で渡す（差分を抑制するため）。  
  1) `<BRAND>_VERSION`  
  2) `GIT_SHA`  
  3) `BUILD_DATE`  
  4) `IMAGE_RUNTIME`  
  5) `COMPONENT`  
- labels は build args の後に渡す。  

#### 19.4.3 buildDockerImage の呼び出し例（擬似）
```
args := []string{
  "--build-arg", "<BRAND>_VERSION="+request.Version,
  "--build-arg", "GIT_SHA="+os.Getenv("GIT_SHA"),
  "--build-arg", "BUILD_DATE="+os.Getenv("BUILD_DATE"),
  "--build-arg", "IMAGE_RUNTIME=containerd",
  "--build-arg", "COMPONENT=agent",
}
```

### 19.5 agent の関数イメージ解決
対象:
- `services/agent/internal/runtime/image_naming.go`
- `services/agent/internal/runtime/image_naming_test.go`

設計:
- `IMAGE_PREFIX` の環境変数参照を削除し、`meta.ImagePrefix` 固定にする。
- これにより関数イメージ名は `meta.ImagePrefix` に完全追随する。

#### 19.5.1 関数イメージ名の最終形
- `meta.ImagePrefix + "-" + <function-image-name>` を固定形とする。  
- `<function-image-name>` は `imageSafeName` の出力を使用する。  

### 19.6 Runtime Guard の実装
対象:
- `services/agent/entrypoint.sh`
- `services/gateway/entrypoint.sh`
- `services/runtime-node/entrypoint.containerd.sh`
- `services/runtime-node/entrypoint.firecracker.sh`
 - `services/runtime-node/entrypoint.sh`（新規ラッパー）

設計（擬似コード）:
```
if [ -z "$IMAGE_RUNTIME" ]; then
  echo "ERROR: IMAGE_RUNTIME is required"; exit 1
fi
case "$IMAGE_RUNTIME" in
  docker)
    [ "$AGENT_RUNTIME" = "docker" ] || { echo "ERROR: runtime mismatch"; exit 1; }
    ;;
  containerd)
    [ "$AGENT_RUNTIME" = "containerd" ] || { echo "ERROR: runtime mismatch"; exit 1; }
    ;;
  *)
    echo "ERROR: invalid IMAGE_RUNTIME"; exit 1
    ;;
esac
```
- gateway / provisioner は `IMAGE_RUNTIME` の値検証のみを行う。
- runtime-node は `IMAGE_RUNTIME=containerd` 以外で即終了する。
- 終了コードは `exit 1` に統一し、ログは `ERROR: <reason>` の形式で出力する。

#### 19.6.1 runtime-node entrypoint ラッパー仕様
目的: containerd / firecracker の分岐を **1つの entrypoint** に集約する。  

擬似コード:
```
if [ -z "$IMAGE_RUNTIME" ]; then
  echo "ERROR: IMAGE_RUNTIME is required"; exit 1
fi
if [ "$IMAGE_RUNTIME" != "containerd" ]; then
  echo "ERROR: IMAGE_RUNTIME must be containerd"; exit 1
fi
if [ "$CONTAINERD_RUNTIME" = "aws.firecracker" ]; then
  exec /entrypoint.firecracker.sh "$@"
fi
exec /entrypoint.containerd.sh "$@"
```

必須条件:
- ラッパーは `IMAGE_RUNTIME` の guard を最初に実行する。
- `CONTAINERD_RUNTIME` が未設定または別値なら containerd 側へ分岐する。
- Compose は常に `entrypoint: /entrypoint.sh` を使用する。
- `RUNTIME_MODE` ベースの既存分岐は廃止する（既存の `entrypoint.sh` を置換）。

#### 19.6.2 entrypoint 差し替え手順（明示）
- `services/runtime-node/entrypoint.sh` を新仕様（IMAGE_RUNTIME + CONTAINERD_RUNTIME 分岐）に置換。  
- 既存の `RUNTIME_MODE` 分岐は削除。  
- `ENTRYPOINT [\"/entrypoint.sh\"]` は維持。  

### 19.7 Dockerfile の整理
対象:
- `services/*/Dockerfile*`

設計:
- `ARG IMAGE_PREFIX=<brand>` のような固定デフォルトを廃止。
- `IMAGE_RUNTIME` / `COMPONENT` / `<BRAND>_VERSION` を `ENV` に焼き込む。
- `IMAGE_PREFIX` はビルド時に明示的に渡す（外部入力ではない）。
- 2系統（docker / containerd）の Dockerfile を用意する。

### 19.8 OCI ラベル
対象:
- `cli/internal/generator/go_builder_helpers.go`
- `cli/internal/compose/docker.go`

設計:
- `meta.LabelPrefix` を使用し、`com.<brand>.*` のラベルを付与する。
- 既存の label キー名は保持し、値のみブランドに追随させる。

### 19.9 containerd / firecracker 切替
対象:
- `services/runtime-node/entrypoint.containerd.sh`
- `services/runtime-node/entrypoint.firecracker.sh`
 - `services/runtime-node/entrypoint.sh`（新規ラッパー）

設計:
- `CONTAINERD_RUNTIME=aws.firecracker` の場合は **entrypoint ラッパー**が firecracker 用を実行する。
- それ以外は containerd 用を実行する。
- ラッパーは `IMAGE_RUNTIME` の guard を最初に実行した後に分岐する。
- Compose は常に `entrypoint: /entrypoint.sh` を使用する。

### 19.10 WireGuard 条件
対象:
- `services/gateway/entrypoint.sh`
- `services/runtime-node/entrypoint.common.sh`

設計:
- gateway: `WG_CONF_PATH` が存在する場合のみ起動。
- runtime-node: `WG_CONTROL_NET` が指定された場合のみルート設定。

### 19.11 変更チェックリスト（ファイル単位）
#### CLI / Generator
- `cli/internal/commands/build.go`  
  - `<BRAND>_VERSION` を解決して `BuildRequest.Version` に設定。未設定は即エラー。  
- `cli/internal/workflows/build.go`  
  - `BuildRequest` に `Version` を追加し、generator へ伝播。  
- `cli/internal/generator/build_request.go`  
  - `BuildRequest.Version` を追加。  
- `cli/internal/helpers/env_defaults.go`  
  - `IMAGE_TAG` / `IMAGE_PREFIX` の設定を削除。  
  - `ENV_PREFIX` 未設定時は即失敗。  
  - `GIT_SHA` / `BUILD_DATE` を一度だけ決定し保持。  
- `cli/internal/generator/go_builder.go`  
  - `resolveImageTag(request.Env)` を削除し `request.Version` を使用。  
  - `resolveImageTag` の `error` を処理。  
- `cli/internal/generator/go_builder_helpers.go`  
  - `resolveImageTag(env string)` → `resolveImageTag(version string) (string, error)`  
  - `resolveRegistryConfig(mode string)` → `resolveRegistryConfig()`  
  - registry の自動生成は廃止（`<BRAND>_REGISTRY` のみ）。  
- `cli/internal/generator/templates/functions.yml.tmpl`  
  - `IMAGE_TAG` / `IMAGE_PREFIX` / `FUNCTION_IMAGE_PREFIX` を使用しない。  
  - 完全な `image` 文字列を出力する。  
- `cli/internal/generator/renderer.go`  
  - `ImagePrefix` は `meta.ImagePrefix` 固定。  
  - `Registry` 正規化（末尾 `/` 付与）。  
- `cli/internal/generator/renderer_test.go` / `testdata/*.golden`  
  - 期待値を新しい `image` 文字列に更新。  

#### Services
- `services/agent/entrypoint.sh`  
  - `IMAGE_RUNTIME` / `AGENT_RUNTIME` guard を追加。  
- `services/gateway/entrypoint.sh`  
  - `IMAGE_RUNTIME` guard を追加。  
- `services/runtime-node/entrypoint.sh`  
  - `RUNTIME_MODE` 分岐を廃止し、新仕様のラッパーに置換。  
- `services/runtime-node/entrypoint.containerd.sh` / `entrypoint.firecracker.sh`  
  - guard 前提で動作する前提に整理。  
- `services/agent/internal/runtime/image_naming.go`  
  - `IMAGE_PREFIX` 参照を削除し `meta.ImagePrefix` 固定。  

#### Compose / Config
- `docker-compose.docker.yml` / `docker-compose.containerd.yml` / `docker-compose.fc.yml`  
  - `IMAGE_TAG` / `FUNCTION_IMAGE_PREFIX` / `IMAGE_PREFIX` を廃止。  
  - `<BRAND>_REGISTRY` / `<BRAND>_TAG` のみに統一。  
- `config/defaults.env`  
  - `IMAGE_PREFIX` の固定値は削除（branding 生成に依存）。  

#### Runtime-node Dockerfile
- `services/runtime-node/Dockerfile` / `Dockerfile.firecracker`  
  - `ENTRYPOINT` は `/entrypoint.sh` を維持。  
  - `ARG IMAGE_PREFIX=<brand>` の固定値を撤去。  

#### E2E
- `e2e/runner/env.py`  
  - `IMAGE_TAG` / `IMAGE_PREFIX` の計算を廃止。  
  - `<BRAND>_TAG` / `<BRAND>_REGISTRY` のみを外部入力として扱う。  
- `e2e/runner/constants.py`  
  - `ENV_IMAGE_TAG` / `ENV_IMAGE_PREFIX` を撤去。  
- `e2e/runner/test_env.py`  
  - `IMAGE_TAG` / `IMAGE_PREFIX` の期待値を削除。  

## 20. E2E テスト修正計画（必須）
### 20.1 目的
- 新しい命名規則と外部入力の最小化が E2E でも一貫していることを保証する。
- runtime guard と WireGuard 条件が期待通りに動作することを検証する。

### 20.2 影響範囲（更新対象）
- E2E ランナーの環境変数生成:
  - `<BRAND>_REGISTRY` / `<BRAND>_TAG` のみを外部入力として扱う。
  - `IMAGE_TAG` / `IMAGE_PREFIX` / `FUNCTION_IMAGE_PREFIX` 前提を撤去する。
- 画像名の期待値:
  - `<brand>-<component>-{docker|containerd}` を前提に期待値を更新する。
- compose / 起動プロファイル:
  - docker / containerd の2系統で E2E シナリオを整理する。
  - firecracker は containerd 系統の runtime 切替で検証する。
  - entrypoint ラッパーの分岐が反映される起動方法に統一する。

### 20.3 修正内容（実装指針）
1) E2E で使用している環境変数を棚卸しする。
2) 外部入力を `<BRAND>_REGISTRY` / `<BRAND>_TAG` のみに揃える。
3) 画像名の期待値を `<brand>-<component>-{docker|containerd}` に置換する。
4) containerd 系統のケースで `CONTAINERD_RUNTIME=aws.firecracker` を付与し、firecracker 相当のケースを再現する。
5) 旧 `IMAGE_TAG` 前提が残る場合はすべて廃止する。

### 20.4 追加・変更テストケース
- runtime guard:
  - `IMAGE_RUNTIME=docker` で `AGENT_RUNTIME=containerd` を与えた場合に起動が失敗すること。
  - `IMAGE_RUNTIME=containerd` で `AGENT_RUNTIME=docker` を与えた場合に起動が失敗すること。
- WireGuard 条件:
  - `WG_CONF_PATH` が存在しない場合に gateway が起動し続けること。
  - `WG_CONTROL_NET` が未指定の場合に runtime-node が起動し続けること。

### 20.5 完了条件
- すべての E2E プロファイルが新命名規則で成功する。
- 外部入力の変数が `<BRAND>_REGISTRY` / `<BRAND>_TAG` のみに統一されている。

### 20.6 E2E 修正チェックリスト（具体）
#### 変更対象（必須）
- `e2e/runner/env.py`  
  - `IMAGE_TAG` / `IMAGE_PREFIX` の生成と注入を削除。  
  - `<BRAND>_TAG` / `<BRAND>_REGISTRY` を環境から取得し、未設定は `latest` を使用。  
- `e2e/runner/constants.py`  
  - `ENV_IMAGE_TAG` / `ENV_IMAGE_PREFIX` を削除。  
- `e2e/runner/test_env.py`  
  - `IMAGE_TAG` / `IMAGE_PREFIX` に関する期待値を削除または置換。  

### 19.12 差分サンプル（代表例）
#### functions.yml テンプレート
変更前:
```
image: "${FUNCTION_IMAGE_PREFIX}${IMAGE_PREFIX}-{{ .ImageName }}:${IMAGE_TAG}"
```

変更後:
```
image: "{{ .Registry }}{{ .ImagePrefix }}-{{ .ImageName }}:{{ .Tag }}"
```

#### generator テスト期待値
変更前:
```
${FUNCTION_IMAGE_PREFIX}${IMAGE_PREFIX}-lambda-hello:${IMAGE_TAG}
```

変更後（例）:
```
<registry>/<brand>-lambda-hello:vX.Y.Z
```

#### runtime-node entrypoint 分岐
変更前（概略）:
```
mode="${RUNTIME_MODE:-containerd}"
case "$mode" in
  firecracker|fc) exec /entrypoint.firecracker.sh ;;
  containerd|"") exec /entrypoint.containerd.sh ;;
  *) exit 1 ;;
esac
```

変更後（概略）:
```
if [ "$IMAGE_RUNTIME" != "containerd" ]; then exit 1; fi
if [ "$CONTAINERD_RUNTIME" = "aws.firecracker" ]; then
  exec /entrypoint.firecracker.sh
fi
exec /entrypoint.containerd.sh
```

### 19.13 具体変更点（関数/定数/インターフェース）
#### 19.13.1 `envutil` の仕様変更
- `HostEnvKey(suffix string)` は **ENV_PREFIX 未設定時にエラー**を返す仕様に変更。  
  - 例: `HostEnvKey` を `func HostEnvKey(suffix string) (string, error)` に変更。  
- `GetHostEnv` / `SetHostEnv` は `error` を返すように変更し、  
  `applyRuntimeEnv` 側で即失敗させる。  

#### 19.13.2 `RuntimeEnvApplier` のエラー伝播
- `cli/internal/ports/env.go` の `RuntimeEnvApplier` を `Apply(ctx state.Context) error` に変更。  
- `helpers.NewRuntimeEnvApplier` は `error` を返す実装に修正。  
- `BuildWorkflow.Run` は `EnvApplier.Apply` のエラーを上位に返す。  

#### 19.13.3 `constants/env.go` の削除対象
- `EnvImageTag` / `EnvImagePrefix` / `HostSuffixImageTag` を削除。  
- 参照箇所はすべて削除または `<BRAND>_TAG` / `<BRAND>_REGISTRY` に置換。  

#### 19.13.4 `BuildRequest` の最終形
- `cli/internal/workflows.BuildRequest` / `cli/internal/generator.BuildRequest` に  
  `Version string` を追加。  
- `BuildRequest.Version` は `<BRAND>_VERSION` 由来のみ。  

### 19.14 差分サンプル（具体ファイル）
#### `cli/internal/envutil/envutil.go`（概略）
変更前:
```
func HostEnvKey(suffix string) string
```

変更後:
```
func HostEnvKey(suffix string) (string, error)
```

#### `cli/internal/ports/env.go`（概略）
変更前:
```
Apply(ctx state.Context)
```

変更後:
```
Apply(ctx state.Context) error
```

### 19.15 変更適用の順序（推奨）
1) `envutil` の関数シグネチャ変更  
2) `RuntimeEnvApplier` のインターフェース変更  
3) `applyRuntimeEnv` のエラーチェック追加と `ENV_PREFIX` 必須化  
4) `<BRAND>_VERSION` の解決と `BuildRequest.Version` 追加  
5) `resolveImageTag` / `resolveRegistryConfig` の置換  
6) generator テンプレートとテストの更新  
7) entrypoint ラッパー置換と runtime guard 実装  
8) compose の環境変数整理  
9) E2E 更新  

### 19.16 影響範囲一覧（呼び出し元）
#### `envutil.HostEnvKey` の呼び出し元
- `cli/internal/config/repo.go`
- `cli/internal/envutil/envutil.go`
- `cli/internal/generator/go_builder_test.go`

#### `RuntimeEnvApplier` の呼び出し元
- `cli/internal/commands/build.go`
- `cli/internal/workflows/build.go`

#### `resolveImageTag` / `resolveRegistryConfig` の呼び出し元
- `cli/internal/generator/go_builder.go`

#### `envutil.GetHostEnv` の呼び出し元（主な箇所）
- `cli/internal/helpers/env_defaults.go`
- `cli/internal/helpers/mode.go`
- `cli/internal/config/global.go`
- `cli/internal/config/repo.go`
- `cli/internal/generator/build_env.go`
- `cli/internal/generator/go_builder_helpers.go`

#### 追加テスト（推奨）
- `<BRAND>_VERSION` 未設定時に CLI が失敗すること。  
- `IMAGE_RUNTIME` mismatch で entrypoint が失敗すること。  
