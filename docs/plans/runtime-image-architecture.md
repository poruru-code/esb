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

## 18. 実装計画（フェーズ分割）
### 18.1 Phase 0: 影響範囲の棚卸し
- 対象ファイルと環境変数の洗い出し（IMAGE_* / 旧プレフィクスの残存確認）。
- branding 生成値（meta.EnvPrefix / meta.ImagePrefix / meta.LabelPrefix）の利用箇所を整理。
受け入れ条件:
- 影響範囲一覧が完成し、変更対象が確定している。

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
受け入れ条件:
- すべてのサービスイメージに `IMAGE_RUNTIME` と `COMPONENT` が入っている。
- ブランド固定のデフォルト値が残っていない。

### 18.5 Phase 4: Runtime Guard 実装
- entrypoint に `IMAGE_RUNTIME` と `AGENT_RUNTIME` の整合チェックを追加。
- 不一致時は明確なエラーで即終了。
受け入れ条件:
- 不一致条件で必ず起動失敗する。
- 一致条件では従来通りに起動する。

### 18.6 Phase 5: containerd / firecracker 切替の統一
- `CONTAINERD_RUNTIME=aws.firecracker` のみで切替できることを保証。
- firecracker モードでは containerd 画像を流用し、entrypoint を切替。
受け入れ条件:
- containerd / firecracker どちらでも同一イメージが使える。

### 18.7 Phase 6: E2E 更新
- E2E で新しい命名規則と外部入力のみを使用。
- firecracker 相当は `CONTAINERD_RUNTIME=aws.firecracker` で再現。
受け入れ条件:
- すべての E2E プロファイルが成功する。

## 19. 詳細設計（コードレベル）
### 19.1 環境変数の解決方法
- 外部入力は `<BRAND>_REGISTRY` / `<BRAND>_TAG` のみ。
- `<BRAND>` は `meta.EnvPrefix` から動的に生成する。
- `envutil.HostEnvKey` は `ENV_PREFIX` を前提にし、固定デフォルトは使わない。

### 19.2 CLI の環境反映
対象:
- `cli/internal/helpers/env_defaults.go`
- `cli/internal/envutil/envutil.go`

設計:
- `applyRuntimeEnv` の先頭で `applyBrandingEnv` を実行し、`ENV_PREFIX` を必ず先に設定する。
- `IMAGE_TAG` / `IMAGE_PREFIX` を設定する処理を削除する。
- `<BRAND>_TAG` が未設定の場合は `latest` を使用（開発用途のみ想定）。

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

### 19.4 サービスイメージの命名とビルド
対象:
- `cli/internal/generator/go_builder.go`
- `cli/internal/generator/go_builder_helpers.go`
- 各 `docker-compose.*.yml`

設計:
- サービスイメージ名は `<brand>-<component>-{docker|containerd}` に固定。
- Compose は `<BRAND>_REGISTRY` / `<BRAND>_TAG` だけ参照する。
- `IMAGE_TAG` / `FUNCTION_IMAGE_PREFIX` / `IMAGE_PREFIX` は Compose から削除する。

### 19.5 agent の関数イメージ解決
対象:
- `services/agent/internal/runtime/image_naming.go`
- `services/agent/internal/runtime/image_naming_test.go`

設計:
- `IMAGE_PREFIX` の環境変数参照を削除し、`meta.ImagePrefix` 固定にする。
- これにより関数イメージ名は `meta.ImagePrefix` に完全追随する。

### 19.6 Runtime Guard の実装
対象:
- `services/agent/entrypoint.sh`
- `services/gateway/entrypoint.sh`
- `services/runtime-node/entrypoint.containerd.sh`
- `services/runtime-node/entrypoint.firecracker.sh`

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

設計:
- `CONTAINERD_RUNTIME=aws.firecracker` の場合は firecracker 用 entrypoint を使用。
- それ以外は containerd 用 entrypoint を使用。

### 19.10 WireGuard 条件
対象:
- `services/gateway/entrypoint.sh`
- `services/runtime-node/entrypoint.common.sh`

設計:
- gateway: `WG_CONF_PATH` が存在する場合のみ起動。
- runtime-node: `WG_CONTROL_NET` が指定された場合のみルート設定。

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
