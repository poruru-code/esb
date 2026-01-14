# `esb completion` コマンド

## 概要

`esb completion` コマンドは、Bash, Zsh, Fish用のシェル補完スクリプトを生成します。これらのスクリプトにより、サブコマンド、フラグ、および環境名、プロジェクト名、サービス名などの動的な値に対するタブ補完が可能になります。

## 使用方法

```bash
esb completion [shell]
```

### サブコマンド

| コマンド | 説明 |
|----------|------|
| `bash` | Bash用補完スクリプトを生成します。 |
| `zsh` | Zsh用補完スクリプトを生成します。 |
| `fish` | Fish用補完スクリプトを生成します。 |

## 実装詳細

コマンドのロジックは `cli/internal/app/completion.go` に実装されています。

### 動的補完ロジック

生成されたスクリプトは、隠しコマンドである `esb __complete` を呼び出して、実行時に動的な候補を取得します。

- `__complete env`: `generator.yml` から利用可能な環境をリストします。
- `__complete project`: グローバル設定から登録済みプロジェクトをリストします。
- `__complete service`: Dockerサービスをリストします（`logs` および `env var` 用）。

### Bash 実装
- `compgen` と case 文を使用してコンテキストを処理します。
- ヘルパー関数 `_esb_find_index` と `_esb_has_positional_after` がコマンドに対するカーソル位置を追跡します。

### Zsh 実装
- より豊富な説明を提供するために `compdef` と `_values` を使用します。
- 動的リストについては `esb __complete` に委譲します。

### Fish 実装
- `complete -c esb ... -a '(esb __complete ...)'` を使用します。
- 正しいコンテキスト（例: `use` や `remove` の後）でのみ補完がトリガーされるように条件を定義します。
