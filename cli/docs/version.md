# `esb version` コマンド

## 概要

`esb version` コマンドは、CLIの現在のバージョンを表示します。

## 使用方法

```bash
esb version
```

## 実装詳細

- **場所**: `cli/internal/command/app.go` (`runVersion`).
- **ロジック**: `version.GetVersion()` が返す文字列を表示します。
- **ビルド時**: バージョン情報は通常、ビルドプロセス中にリンカーフラグ (`-ldflags`) を介して注入されます（`cli/version/version.go` 等で処理）。

## フローチャート

```mermaid
flowchart LR
    Start([esb version]) --> GetVer[version.GetVersion]
    GetVer --> Print[標準出力へ表示]
    Print --> End([終了])
```
