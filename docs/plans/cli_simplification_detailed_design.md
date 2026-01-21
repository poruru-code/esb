# CLI 簡素化 詳細設計書 (Level: Low)

## 1. 概要
`esb` CLI をリファクタリングし、`docker compose` と重複する機能を削除する。
E2E テスト用の環境変数準備ロジックは CLI から除外し、テストランナー (Python) 側に移植する。CLI は SAM テンプレートからのデプロイメント（プロビジョニング）に集中する。

**注記**: `esb build` (アーティファクト生成) および `esb project` (プロジェクト管理) コマンドは本リファクタリングの対象外であり、機能変更なしで維持される。

## 2. 変更対象ファイル一覧 (Files to Modify/Create)

| Action     | File Path                                            | Description                                              |
| :--------- | :--------------------------------------------------- | :------------------------------------------------------- |
| **New**    | `cli/internal/commands/sync.go`                      | `esb sync` 実装 (ポート検出＆プロビジョニング)           |
| **Modify** | `cli/internal/commands/app.go`                       | 不要コマンド削除 (`Up`, `Down`, `Logs`, `Stop`, `Prune`) |
| **Delete** | `cli/internal/commands/{up,down,logs,stop,prune}.go` | 実装ファイル削除                                         |
| **Modify** | `e2e/runner/executor.py`                             | CLI呼び出しフロー変更 (docker compose 直接実行)          |
| **Modify** | `e2e/runner/env.py`                                  | 環境変数計算ロジックを Go から移植                       |
| **Delete** | `cli/internal/helpers/env_defaults.go`               | ロジック移植後、不要になれば削除 (または縮小)            |

## 3. 実装詳細

### 3.1 Logic Migration: `cli/internal/helpers/` -> `e2e/runner/env.py`

**方針**: 動的なポート割り当て、サブネット計算、シークレット生成などのロジックは CLI から削除し、E2E ランナー（Python）に移植する。
本番環境や手動開発環境では、ユーザーが静的な `.env` を用意する前提とする。

**移植対象ロジック (Go -> Python)**:
- **Port Defaults**: 環境変数が未設定なら "0" をセット。
- **Subnet Defaults**: 環境名ハッシュからサブネット計算 (`172.x.0.0/16`).
- **Registry Defaults**: モードに応じたレジストリ設定 (`registry:5010`).
- **Credential Generation**: `AUTH_USER`, `JWT_SECRET` などの生成。

### 3.2 Command: `esb sync`

**File**: `cli/internal/commands/sync.go` (新規)

**目的**: `docker compose up` で起動したコンテナに対して、SAM テンプレートに基づいたリソース（DynamoDB テーブル、S3 バケット）をプロビジョニングする。また、Docker API を使用して公開ポートを検出し、`ports.json` を生成する。

#### [Struct]
```go
type SyncCmd struct {
    Wait bool `help:"Wait for services to be ready" default:"true"`
}
```

#### [Function] `runSync`
```go
func runSync(cli CLI, deps Dependencies, out io.Writer) int {
    // 1. Context Resolution
    opts := newResolveOptions(false)
    ctx, err := resolveCommandContext(cli, deps, opts)
    if err != nil {
         return exitWithError(out, err)
    }

    // 2. Port Discovery
    // helpers.NewPortDiscoverer を使用 (Docker API)
    discoverer := helpers.NewPortDiscoverer(deps.DockerClient)
    ports, err := discoverer.Discover(ctx.ComposeProject, helpers.DefaultPorts)
    if err != nil {
        // エラーハンドリング
    }
    
    // 3. Write ports.json
    // E2E ランナーが参照するパス (~/.esb/<project>/<env>/ports.json) に書き出す
    configDir := staging.ConfigDir(ctx.Project.Name, ctx.Env)
    if err := os.MkdirAll(configDir, 0755); err != nil {
        return exitWithError(out, err)
    }
    
    portsParams := make(map[string]int)
    for k, v := range ports {
         portsParams[k] = v
    }
    jsonBytes, _ := json.MarshalIndent(portsParams, "", "  ")
    if err := os.WriteFile(filepath.Join(configDir, "ports.json"), jsonBytes, 0644); err != nil {
        return exitWithError(out, err)
    }
    
    // 4. Provisioning (DynamoDB / S3)
    if deps.Provisioner != nil {
         // manifest parsing (SAM template)
         m, err := manifest.Load(ctx.Project.Generator.Paths.SamTemplate)
         if err == nil {
             deps.Provisioner.Apply(context.Background(), m.Resources, ctx.ComposeProject)
         }
    }

    fmt.Fprintln(out, "Sync complete")
    return 0
}
```

### 3.3 Cleanup: `app.go` & Deleted Commands

**File**: `cli/internal/commands/app.go`
- `Up`, `Down`, `Logs`, `Stop`, `Prune` フィールド削除。
- `dispatchCommand` 内の case 文削除。

**Files to Delete**:
- `cli/internal/commands/up.go`
- `cli/internal/commands/down.go`
- `cli/internal/commands/logs.go`
- `cli/internal/commands/stop.go`
- `cli/internal/commands/prune.go`

## 4. E2E テストランナー修正仕様

### 4.1 `e2e/runner/env.py` (New Logic)

Python 側で環境変数を計算する関数を追加する。

```python
def calculate_runtime_env(project_name: str, env_name: str, mode: str) -> dict:
    env = os.environ.copy()
    
    # 1. Basic Metadata
    env["COMPOSE_PROJECT_NAME"] = project_name
    env["ENV"] = env_name
    env["MODE"] = mode
    
    # 2. Port Defaults (0 for dynamic)
    for port in ["GATEWAY_PORT_HTTPS", "DATABASE_PORT", ...]:
        if port not in env:
            env[port] = "0"

    # 3. Subnets (Hash based logic from Go)
    # ... implementation of subnet calculation ...
    
    # 4. Credentials
    # ... generate generic credentials ...
    
    # 5. Compose File Selection
    # Firecracker mode handling
    if mode == "firecracker":
        # Note: Split execution handles this, but here we might set defaults
        pass 
        
    return env
```

### 4.2 `e2e/runner/executor.py`

**`run_scenario` 関数**:

1.  **Environment Preparation**:
    - `env_vars = env.calculate_runtime_env(...)` を呼び出し。

2.  **Reset / Cleanup**:
    - `subprocess.run(["docker", "compose", "down", "-v", ...], env=env_vars)`

3.  **Up**:
    - `subprocess.run(["docker", "compose", "up", "-d", "--wait"], env=env_vars)`
    - Firecracker モードの場合、`--target` ごとに分けて `up` を呼ぶ制御が必要になる可能性があるが、Executor は基本的に `docker-compose.fc.yml` 等を連結して呼ぶか、シナリオ側で制御する。
    - *補足*: ユーザーフィードバックに基づき、`fc` と `fc-node` が別マシン前提であれば、E2Eテストはローカルで両方起動するために `COMPOSE_FILE=docker-compose.fc.yml:docker-compose.fc-node.yml` として結合して起動するのが最もシンプル。

4.  **Sync**:
    - `run_esb(["sync"], ...)`

## 5. テスト計画 (Validation)

### 5.1 Unit Tests
- `cli/internal/commands/sync_test.go` (New):
    - `ports.json` の生成確認。
    - Provisioner 呼び出し確認。
- `e2e/tests/test_env.py` (New):
    - Python に移植した環境変数計算ロジックのテスト。

### 5.2 Manual Verification
1. E2E テストがパスすることを確認。
2. 古いコマンドが CLI から消えていることを確認。
