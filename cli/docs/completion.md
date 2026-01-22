# `esb completion` コマンド

## 概要

`esb completion` コマンドは、Bash, Zsh, Fish用のシェル補完スクリプトを生成します。build-only CLI 向けに、サブコマンドと固定サブコマンド（`completion` の `bash/zsh/fish`）の補完を提供します。

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

コマンドのロジックは `cli/internal/commands/completion.go` に実装されています。動的候補の取得は行わず、`build` / `completion` / `version` の補完と、`completion` サブコマンドの補完のみを提供します。
