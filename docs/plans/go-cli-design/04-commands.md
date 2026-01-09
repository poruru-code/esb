# CLI コマンド

## CLI フロー

```bash
# 1. プロジェクト初期化
$ esb init --template ./template.yaml
Registered project 'myapp'
Created environment 'default'

# 2. 環境追加
$ esb env create staging
Created environment 'staging'

# 3. 環境切り替え
$ esb env use staging
Switched to 'myapp:staging'

# 4. ビルド・起動
$ esb build
$ esb up

# 5. プロジェクト切り替え
$ esb project use another
Switched to project 'another'

# 6. 履歴から復帰
$ esb project recent
  1. myapp      (5 minutes ago)
  2. another    (2 hours ago)

$ esb project use 1
```

## コマンド一覧

### 基本コマンド

| コマンド | 説明 | 許可状態 |
|---------|------|---------|
| `esb init -t <template>` | プロジェクト初期化 | Uninitialized |
| `esb build` | Dockerfile 生成 + イメージビルド | Initialized, Built |
| `esb up [-d] [--build]` | ビルド後にスタック起動 | Built, Stopped |
| `esb down` | スタック停止 | Running |
| `esb stop` | スタック一時停止 (データ保持) | Running |
| `esb reset -y` | 環境再構築 (down -v → build → up) | Initialized+ |
| `esb status` | 状態表示 | 全て |
| `esb info` | 設定/状態の表示 | 全て |
| `esb logs [-f] [service]` | ログ表示 | Running |
| `esb prune [-y] [--hard]` | コンテナ/ボリューム破棄 + 生成物削除 | Initialized+ |

### 環境管理

| コマンド | 説明 |
|---------|------|
| `esb env list` | 環境一覧 |
| `esb env create <name>` | 環境作成 |
| `esb env use <name>` | 環境切り替え |
| `esb env remove <name>` | 環境削除 |

### プロジェクト管理

| コマンド | 説明 |
|---------|------|
| `esb project list` | プロジェクト一覧 |
| `esb project use <name|index>` | プロジェクト切り替え |
| `esb project recent` | 最近のプロジェクト |

## CLI 構造体 (Kong)

```go
type CLI struct {
    Template string `short:"t" help:"Path to SAM template"`
    EnvFlag string `short:"e" name:"env" help:"Environment (default: active)"`

    Init    InitCmd    `cmd:"" help:"Initialize project"`
    Build   BuildCmd   `cmd:"" help:"Build images"`
    Up      UpCmd      `cmd:"" help:"Start environment"`
    Down    DownCmd    `cmd:"" help:"Stop environment"`
    Reset   ResetCmd   `cmd:"" help:"Reset environment"`
    Stop    StopCmd    `cmd:"" help:"Stop (preserve state)"`
    Logs    LogsCmd    `cmd:"" help:"View logs"`
    Status  StatusCmd  `cmd:"" help:"Show state"`
    Info    InfoCmd    `cmd:"" help:"Show configuration and state"`
    Prune   PruneCmd   `cmd:"" help:"Remove resources"`

    Env     EnvCmd     `cmd:"" name:"env" help:"Manage environments"`
    Project ProjectCmd `cmd:"" help:"Manage projects"`
}

type InitCmd struct {
    Name     string `short:"n" help:"Project name (default: directory)"`
}

type UpCmd struct {
    Build  bool `help:"Rebuild before starting"`
    Detach bool `short:"d" default:"true" help:"Run in background"`
    Wait   bool `short:"w" help:"Wait for gateway ready"`
}

補足: `esb up --build` は `esb build` を先に実行するだけで、`docker compose --build` は使用しない。

type StopCmd struct {}

type LogsCmd struct {
    Service    string `arg:"" optional:"" help:"Service name (default: all)"`
    Follow     bool   `short:"f" help:"Follow logs"`
    Tail       int    `help:"Tail the latest N lines"`
    Timestamps bool   `help:"Show timestamps"`
}

type ResetCmd struct {
    Yes bool `short:"y" help:"Skip confirmation"`
}

type PruneCmd struct {
    Yes  bool `short:"y" help:"Skip confirmation"`
    Hard bool `help:"Also remove generator.yml"`
}
```
