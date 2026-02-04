# `esb completion` コマンド

## 概要
`esb completion` は Bash / Zsh / Fish 用の補完スクリプトを標準出力に生成します。
補完対象は **静的なコマンド構造のみ**で、動的候補は生成しません。

## 使用方法

```bash
esb completion <bash|zsh|fish>
```

### 例

```bash
# Bash
source <(esb completion bash)

# Zsh
source <(esb completion zsh)

# Fish
esb completion fish | source
```

## 対応コマンド
- `deploy`
- `completion` (`bash` / `zsh` / `fish`)
- `version`

## 実装詳細
- 実装: `cli/internal/command/completion.go`
- Kong のコマンドモデルからサブコマンド一覧を取得
- `CLI_CMD` 環境変数で CLI 名を変更している場合も反映
