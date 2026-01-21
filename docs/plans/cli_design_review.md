# CLI 簡素化設計レビュー (Rev. 2)

本書は、`esb env prepare` を廃止し、環境構築ロジックをテストランナーへ委譲するという修正案に対する、客観的なソフトウェアアーキテクトによる5回にわたる厳格なレビュー結果です。

## 第1回: アーキテクチャの明確性とCLIのスコープ

### 目的
CLI の責任範囲の変化と、それがシステム全体のアーキテクチャに与える影響を評価する。

### 調査結果
1.  **関心事の完全な分離**: `esb` CLI は「SAMテンプレートに基づくリソース作成 (Provisioning)」と「状態の同期 (Sync)」のみに集中することになった。これは "Do one thing and do it well" の原則に合致する。環境変数計算という「設定の準備」を CLI から切り離したのは英断である。
2.  **テストランナーへの委譲**: E2E テストに必要な複雑な環境変数パズル（ポート計算、サブネットハッシュ計算など）を、テストを実行する主体である Python ランナーに移動させたことは論理的である。テストの設定はテストコードの近くにあるべきだ。
3.  **本番環境との整合性**: ユーザーフィードバック「本番環境は予め用意されたenvファイルを使用する」に基づき、CLI に環境構築機能を持たせないという判断は、本番と開発のギャップを埋めるものではなく、むしろ「CLI は余計なことをしない」という姿勢を明確にしている。

### スコア: 9/10
アーキテクチャは前回よりはるかにクリーンである。CLI は純粋なユーティリティとなり、オーケストレーションの責任から解放された。

---

## 第2回: 開発者体験 (DX) とユーザビリティ

### 目的
新しいワークフローが、日々の開発業務（特に新規参画者やローカル開発）に与える影響を評価する。

### 調査結果
1.  **「魔法」の消失**: 以前の ✗ no projects registered; run 'esb project add . --template <path>' to get started はワンコマンドで全てを整えてくれていたが、今後は開発者が自分で  を用意するか、提供されるスクリプト (E2Eランナー等) を使う必要がある。
    - *批判*: 初学者が  を見て「どの環境変数を設定すればいいの？」と迷うリスクがある。 に隠蔽されていた知識（サブネット計算など）が、Python コードに移ったことで、ドキュメント化されていないと再利用しにくい。
    - *緩和策*:  や  に、開発用の標準  の例を記載することが必須となる。
2.  **プロセスの透明性**: 逆に、Usage:  docker compose [OPTIONS] COMMAND

Define and run multi-container applications with Docker

Options:
      --all-resources              Include all resources, even those not used by services
      --ansi string                Control when to print ANSI control characters ("never"|"always"|"auto") (default "auto")
      --compatibility              Run compose in backward compatibility mode
      --dry-run                    Execute command in dry run mode
      --env-file stringArray       Specify an alternate environment file
  -f, --file stringArray           Compose configuration files
      --parallel int               Control max parallelism, -1 for unlimited (default -1)
      --profile stringArray        Specify a profile to enable
      --progress string            Set type of progress output (auto, tty, plain, json, quiet)
      --project-directory string   Specify an alternate working directory
                                   (default: the path of the, first specified, Compose file)
  -p, --project-name string        Project name

Management Commands:
  bridge      Convert compose files into another model

Commands:
  attach      Attach local standard input, output, and error streams to a service's running container
  build       Build or rebuild services
  commit      Create a new image from a service container's changes
  config      Parse, resolve and render compose file in canonical format
  cp          Copy files/folders between a service container and the local filesystem
  create      Creates containers for a service
  down        Stop and remove containers, networks
  events      Receive real time events from containers
  exec        Execute a command in a running container
  export      Export a service container's filesystem as a tar archive
  images      List images used by the created containers
  kill        Force stop service containers
  logs        View output from containers
  ls          List running compose projects
  pause       Pause services
  port        Print the public port for a port binding
  ps          List containers
  publish     Publish compose application
  pull        Pull service images
  push        Push service images
  restart     Restart service containers
  rm          Removes stopped service containers
  run         Run a one-off command on a service
  scale       Scale services 
  start       Start services
  stats       Display a live stream of container(s) resource usage statistics
  stop        Stop services
  top         Display the running processes
  unpause     Unpause services
  up          Create and start containers
  version     Show the Docker Compose version information
  volumes     List volumes
  wait        Block until containers of all (or specified) services stop.
  watch       Watch build context for service and rebuild/refresh containers when files are updated

Run 'docker compose COMMAND --help' for more information on a command. コマンドを直接叩くことになるため、何が起きているかは明白になる。「ℹ️  Version
   928cb4d

⚙️  Config
   path: /home/akira/.esb/config.yaml

📦 No projects registered.
   Run 'esb project add . -t <template>' to get started. が裏で何をしているかわからない」というストレスは解消される。
3.  **コマンド数の増加**: ✗ no projects registered; run 'esb project add . --template <path>' to get started ->  + ✗ unexpected argument sync。1ステップ増えるが、頻度（コード書き換え -> リロード）を考えると、 は構成変更時のみで良いため、普段のイテレーションは Usage:  docker compose [OPTIONS] COMMAND

Define and run multi-container applications with Docker

Options:
      --all-resources              Include all resources, even those not used by services
      --ansi string                Control when to print ANSI control characters ("never"|"always"|"auto") (default "auto")
      --compatibility              Run compose in backward compatibility mode
      --dry-run                    Execute command in dry run mode
      --env-file stringArray       Specify an alternate environment file
  -f, --file stringArray           Compose configuration files
      --parallel int               Control max parallelism, -1 for unlimited (default -1)
      --profile stringArray        Specify a profile to enable
      --progress string            Set type of progress output (auto, tty, plain, json, quiet)
      --project-directory string   Specify an alternate working directory
                                   (default: the path of the, first specified, Compose file)
  -p, --project-name string        Project name

Management Commands:
  bridge      Convert compose files into another model

Commands:
  attach      Attach local standard input, output, and error streams to a service's running container
  build       Build or rebuild services
  commit      Create a new image from a service container's changes
  config      Parse, resolve and render compose file in canonical format
  cp          Copy files/folders between a service container and the local filesystem
  create      Creates containers for a service
  down        Stop and remove containers, networks
  events      Receive real time events from containers
  exec        Execute a command in a running container
  export      Export a service container's filesystem as a tar archive
  images      List images used by the created containers
  kill        Force stop service containers
  logs        View output from containers
  ls          List running compose projects
  pause       Pause services
  port        Print the public port for a port binding
  ps          List containers
  publish     Publish compose application
  pull        Pull service images
  push        Push service images
  restart     Restart service containers
  rm          Removes stopped service containers
  run         Run a one-off command on a service
  scale       Scale services 
  start       Start services
  stats       Display a live stream of container(s) resource usage statistics
  stop        Stop services
  top         Display the running processes
  unpause     Unpause services
  up          Create and start containers
  version     Show the Docker Compose version information
  volumes     List volumes
  wait        Block until containers of all (or specified) services stop.
  watch       Watch build context for service and rebuild/refresh containers when files are updated

Run 'docker compose COMMAND --help' for more information on a command. のみで完結する可能性がある。これはプラス要因。

### スコア: 7/10
初期セットアップのハードルは若干上がるが、日々の運用における透明性と標準ツールへの回帰は、長い目で見れば開発者の助けになる。

---

## 第3回: 検証とテスタビリティ

### 目的
提案された変更が、品質保証プロセスと自動テストの信頼性にどのように寄与するかを評価する。

### 調査結果
1.  **ホワイトボックス化されたE2E**: E2E ランナーが Usage:  docker compose [OPTIONS] COMMAND

Define and run multi-container applications with Docker

Options:
      --all-resources              Include all resources, even those not used by services
      --ansi string                Control when to print ANSI control characters ("never"|"always"|"auto") (default "auto")
      --compatibility              Run compose in backward compatibility mode
      --dry-run                    Execute command in dry run mode
      --env-file stringArray       Specify an alternate environment file
  -f, --file stringArray           Compose configuration files
      --parallel int               Control max parallelism, -1 for unlimited (default -1)
      --profile stringArray        Specify a profile to enable
      --progress string            Set type of progress output (auto, tty, plain, json, quiet)
      --project-directory string   Specify an alternate working directory
                                   (default: the path of the, first specified, Compose file)
  -p, --project-name string        Project name

Management Commands:
  bridge      Convert compose files into another model

Commands:
  attach      Attach local standard input, output, and error streams to a service's running container
  build       Build or rebuild services
  commit      Create a new image from a service container's changes
  config      Parse, resolve and render compose file in canonical format
  cp          Copy files/folders between a service container and the local filesystem
  create      Creates containers for a service
  down        Stop and remove containers, networks
  events      Receive real time events from containers
  exec        Execute a command in a running container
  export      Export a service container's filesystem as a tar archive
  images      List images used by the created containers
  kill        Force stop service containers
  logs        View output from containers
  ls          List running compose projects
  pause       Pause services
  port        Print the public port for a port binding
  ps          List containers
  publish     Publish compose application
  pull        Pull service images
  push        Push service images
  restart     Restart service containers
  rm          Removes stopped service containers
  run         Run a one-off command on a service
  scale       Scale services 
  start       Start services
  stats       Display a live stream of container(s) resource usage statistics
  stop        Stop services
  top         Display the running processes
  unpause     Unpause services
  up          Create and start containers
  version     Show the Docker Compose version information
  volumes     List volumes
  wait        Block until containers of all (or specified) services stop.
  watch       Watch build context for service and rebuild/refresh containers when files are updated

Run 'docker compose COMMAND --help' for more information on a command. を直接制御することで、テスト環境のセットアップが完全に可視化された。以前の ✗ no projects registered; run 'esb project add . --template <path>' to get started はブラックボックスであり、テスト失敗時に「CLI のバグか、環境の問題か」の切り分けが困難だった。今後は Python コードを追うだけで済む。
2.  **Go コードの責務縮小**:  の複雑なロジックが Go から消えるため、Go 側のユニットテスト負担が激減する。CLI 自体の単体テストは  コマンド（API叩くだけ）などに集中でき、品質を担保しやすくなる。
3.  **Python 側のロジック検証**: 移植されるロジック（サブネット計算など）のテストは Python 側で必要になるが、Python はこの手のデータ処理ロジックのテスト記述に向いている。 の単体テストを書くことは容易である。

### スコア: 9/10
テスタビリティの観点では、不透明な Go バイナリへの依存が減り、スクリプト制御可能な部分が増えたことで大幅な向上が見込まれる。

---

## 第4回: 保守性と将来性

### 目的
コードベースの長期的な健全性と、将来の変更に対する柔軟性を評価する。

### 調査結果
1.  **Duplicate Logic のリスク**: Go 内のロジックを Python に移植するとあるが、CLI にも一部の設定（例えば  時の接続先解決など）が必要な場合、ロジックが重複する恐れがある。
    - *確認*: 設計では  は Docker API 経由でポートを探すため、環境変数からポートを計算する必要はない。よって重複は最小限に抑えられている。
2.  **Go コードの「ダイエット」**:  や  周りの複雑な依存関係が解消されることで、Go コードベースは非常に軽量になる。これは新規参画者がコードを読み解く時間を大幅に短縮する。
3.  **Docker Compose への委譲**: 将来的に Docker Compose の仕様が変わった場合（例:  のバージョンアップ）、CLI 側のコード修正なしに対応できる可能性が高い。CLI が独自にパースやバリデーションを行っていた部分がなくなるため、柔軟性が高まる。

### スコア: 8/10
Go と Python の境界が明確（CLI=Sync, Runner=Env+Exec）である限り、保守性は向上する。コード量の削減効果は大きい。

---

## 第5回: 最終判定

### エグゼクティブサマリー
修正された詳細設計書（✗ unexpected argument prepare 廃止案）は、前回のレビューで見つかった「設定管理の複雑さ」と「UXの摩擦」を効果的に解決している。
CLI の責任範囲を「SAM リソースのプロビジョニング」と「状態同期」に縮小し、環境セットアップをテストランナーやユーザー自身の管理に委ねるアプローチは、UNIX哲学（各プログラムは一つのことをうまくやるべき）に回帰するものであり、アーキテクチャとして非常に健全である。

### 主な改善点
1.  ** 競合の解消**: CLI が  を生成しなくなったため、Firecracker モードで  用と  用の設定が競合するリスクは、運用（ユーザーやスクリプトが別々の  を指定する）の問題として分離された。
2.  **Go 実装の簡素化**: 複雑な環境変数計算ロジックが排除され、Go 実装は非常にシンプルになった。バグの温床が排除された。
3.  **役割の明確化**:
    - **Go (CLI)**: インフラ操作（SAM -> AWSリソース）
    - **Python (Runner)**: テスト環境のオーケストレーション（Env計算 -> Docker操作）
    - **User**: ローカル開発環境の自由な構成

### 最終判定
**Unconditional Approval (無条件承認)**。
この設計はシンプルであり、保守性が高く、テストも容易である。実装を進めることに支障はない。

### 総合スコア: 9/10

---
