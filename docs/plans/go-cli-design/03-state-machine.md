# ステートマシン

## 状態と検出

状態は**ファイルに保存せず、毎回検出**する (Derive, not Store)。  
これにより `docker compose down` を直接実行されても正しい状態を認識できる。

検出は**プロジェクト + 環境にスコープ**し、以下を前提とする:
- generator.yml の `paths.sam_template` と `paths.output_dir` を正として解決
- Docker Compose の project 名 (例: `esb-<env>`) を用いてコンテナを絞り込む
- 生成物 (Dockerfile / functions.yml / routing.yml) の存在をビルド完了の指標にする

プロジェクトは事前に確定している操作体系のため、
Compose project 名は環境単位 (`esb-<env>`) で十分とする。

Docker images の有無は**警告のみ**の補助情報とし、Running/Stopped 判定は
コンテナの実在で優先する。

## 状態図

```
┌─────────────────┐
│  Uninitialized  │ ← generator.yml がない / 不正 / env 未登録
└────────┬────────┘
         │ init
         ▼
┌─────────────────┐
│   Initialized   │ ← generator.yml あり、出力未生成
└────────┬────────┘
         │ build
         ▼
┌─────────────────┐
│      Built      │ ← 出力生成あり、containers なし
└────────┬────────┘
         │ up
         ▼
┌─────────────────┐
│     Running     │ ← containers が running
└────────┬────────┘
         │ stop
         ▼
┌─────────────────┐
│     Stopped     │ ← containers が exited
└─────────────────┘
```

## 検出ロジック

```go
func (d *Detector) Detect() State {
    // 1. generator.yml が存在し、template と env が正しいか
    ctx, err := d.resolveContext() // paths.sam_template, paths.output_dir, env, project
    if err != nil || !ctx.Valid() {
        return StateUninitialized
    }

    // 2. containers の状態 (Compose project 名で絞り込む)
    containers := d.listContainers(ctx.ComposeProject)
    if countRunning(containers) > 0 {
        return StateRunning
    }
    if len(containers) > 0 {
        return StateStopped
    }

    // 3. 出力生成の有無 (Dockerfile + config)
    if d.hasBuildArtifacts(ctx) {
        if !d.hasImages(ctx) {
            d.warnImagesMissing(ctx)
        }
        return StateBuilt
    }

    return StateInitialized
}
```

補足: `hasBuildArtifacts` の判定例
- `<output_dir>/<env>/functions/**/Dockerfile`
- `<output_dir>/<env>/config/functions.yml`
- `<output_dir>/<env>/config/routing.yml`

補足: `hasImages` が false の場合は状態遷移に影響させず、
「イメージが見つからないため `esb build` を推奨」などの警告を出す。

## 遷移制御

```go
var AllowedCommands = map[State][]string{
    StateUninitialized: {"init"},
    StateInitialized:   {"build", "reset", "prune"},
    StateBuilt:         {"up", "build", "reset", "prune"},
    StateRunning:       {"down", "stop", "reset", "logs", "prune"},
    StateStopped:       {"up", "down", "reset", "prune"},
}
```

補足: `status`, `info`, `env`, `project` は状態に依存せず常時許可する。
