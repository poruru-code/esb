# CLI UX 改善計画

## 概要

esb CLI のユーザー体験を改善し、初心者にもわかりやすく、日常的な操作を効率化する。

---

## 1. 引数なし実行で現在地表示

### AS-IS

```bash
$ esb
expected one of "init", "build", "up", "down", "stop", ...
exit status 1

$ esb --help
Usage: esb <command> [flags]

Flags:
  -h, --help               Show context-sensitive help.
  -t, --template=STRING    Path to SAM template
  -e, --env=STRING         Environment (default: last used)

Commands:
  init [flags]      Initialize project
  build [flags]     Build images
  up [flags]        Start environment
  ...more...
```

**問題点**:
- 引数なしで実行するとエラー (`expected one of...`)
- 現在のプロジェクト/環境がわからない
- `exit status 1` が表示される

### TO-BE

```bash
$ esb
esb: my-app:dev (running)

Usage: esb <command>

Quick Actions:
  esb logs     View container logs
  esb down     Stop services

Commands:
  init       Initialize project
  build      Build images
  ...
```

**ポイント**:
- 1行目で現在地を即座に表示
- 状態に応じた Quick Actions を提示

### 実装方針

- `cli/internal/app/app.go` の `Run` 関数で引数なし検出
- 現在地解決 → 簡易ステータス表示 → ヘルプ表示
- 状態に応じて Quick Actions を動的生成

---

## 2. 選択式入力 (引数なしで一覧表示)

### AS-IS

```bash
$ esb env use
expected "<name>"
exit status 1

$ esb project use
expected "<name>"
exit status 1
```

**問題点**:
- Kong のパースエラーで何が選択可能かわからない
- 名前を覚えてタイプする必要がある
- `exit status 1` が表示される

### TO-BE

```bash
$ esb env use
Select environment:
  1) dev (docker) - running
  2) staging (containerd) - stopped
  3) prod (docker) - stopped
> 1
Switched to 'my-app:dev'
export ESB_ENV=dev

$ esb project use
Select project:
  1) my-app (last used: 2 hours ago)
  2) other-project (last used: 3 days ago)
> 1
Switched to project 'my-app'
export ESB_PROJECT=my-app
```

**ポイント**:
- 番号で選択可能
- 状態/最終使用時刻を表示
- TTY でなければ従来通りエラー

### 実装方針

- `env.go` の `runEnvUse` で引数空欄チェック時に分岐
- TTY 判定 → 選択肢表示 → 入力待ち
- `project.go` も同様

---

## 3. エラーメッセージの改善

### AS-IS

```bash
$ esb env use foo
environment not found
exit status 1

$ esb build
No active environment. Run 'esb env use <name>' first.
exit status 1
```

**問題点**:
- 何が存在するかわからない
- `exit status 1` が表示されて初心者には意味不明

### TO-BE

```bash
$ esb env use foo
✗ Environment 'foo' not found.

Available environments:
  - dev (docker)
  - staging (containerd)

$ esb build
✗ No active environment.

Next steps:
  esb env use <name>   # Select environment
  esb env list         # Show available environments

Available:
  - dev
  - staging
```

**ポイント**:
- エラー記号 (✗) で視覚的にわかりやすく
- 選択肢を一覧表示
- 次のアクションを具体的に提示
- `exit status 1` は非表示

### 実装方針

- エラー出力用ヘルパー関数 `exitWithSuggestion(out, err, suggestions, available)` を作成
- Kong の `AfterApply` で exit code 表示を抑制

---

## 4. コマンド成功後のヒント表示

### AS-IS

```bash
$ esb init -t template.yaml --env dev
Initialized project 'my-app' with environment 'dev'

$ esb build
build complete
```

**問題点**:
- 次に何をすべきかわからない

### TO-BE

```bash
$ esb init -t template.yaml --env dev
✓ Initialized project 'my-app' with environment 'dev'

Next steps:
  esb build    # Build container images
  esb up       # Start services

$ esb build
✓ Build complete

Next: esb up   # Start services
```

**ポイント**:
- 成功記号 (✓) で視覚的にわかりやすく
- 次のステップを提示 (状態遷移に基づく)

### 実装方針

- 各コマンドハンドラの return 0 前にヒント出力追加
- 状態遷移マップに基づいて次のアクションを決定

---

## 5. env list で状態表示

### AS-IS

```bash
$ esb env list
e2e-containerd
e2e-docker
e2e-firecracker
```

**問題点**:
- 各環境の状態 (running/stopped) がわからない
- mode (docker/containerd) がわからない
- アクティブ環境のマークなし (last_env 未設定時)

### TO-BE

```bash
$ esb env list
* dev (docker) - running
  staging (containerd) - stopped
  prod (docker) - not built
```

**ポイント**:
- mode と状態を表示
- アクティブ環境に `*` マーク

### 実装方針

- `runEnvList` で各環境の Detector を呼び出し
- 状態取得に失敗した場合は `(unknown)` を表示

---

## 6. Tab 補完サポート

### AS-IS

Tab 補完なし

### TO-BE

```bash
$ esb <TAB>
build   down   env   info   init   logs   project   prune   stop   up

$ esb env use <TAB>
dev   staging   prod
```

### 実装方針

- Kong の `--install-completion` を有効化
- 動的補完のためのシェルスクリプト生成

---

## 優先順位

| # | 改善 | Impact | Effort | 優先度 |
|---|------|--------|--------|-------|
| 1 | 引数なし実行で現在地表示 | High | Low | **P0** |
| 2 | 選択式入力 | High | Medium | **P0** |
| 3 | エラーメッセージ改善 | High | Low | **P0** |
| 4 | 成功後ヒント表示 | Medium | Low | **P1** |
| 5 | env list 状態表示 | Medium | Medium | **P1** |
| 6 | Tab 補完 | Medium | Medium | **P2** |

---

## 実装フェーズ

### Phase 1: 即効性のある改善 (P0)

1. エラーメッセージ改善 + exit status 非表示
2. 引数なし実行で現在地表示
3. 選択式入力 (env use, project use)

### Phase 2: 操作性向上 (P1)

4. 成功後ヒント表示
5. env list 状態表示

### Phase 3: 高度な補完 (P2)

6. Tab 補完サポート
