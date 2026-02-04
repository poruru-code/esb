# `esb version` コマンド

## 概要
`esb version` は CLI のビルド情報からバージョンを表示します。

## 使用方法

```bash
esb version
```

## 実装詳細
- 実装: `cli/internal/version/version.go` / `cli/internal/command/app.go`
- `debug.ReadBuildInfo()` から `vcs.revision` / `vcs.modified` を取得
- リビジョンは **7文字**に短縮
- `vcs.modified=true` の場合は `(<dirty>)` を付与
- ビルド情報が無い場合は `dev` を表示

## フローチャート

```mermaid
flowchart LR
    Start([esb version]) --> ReadInfo[debug.ReadBuildInfo]
    ReadInfo -->|no info| Dev["dev"]
    ReadInfo -->|revision| Shorten[7文字に短縮]
    Shorten --> Dirty{vcs.modified?}
    Dirty -->|yes| PrintDirty["<rev> (dirty)"]
    Dirty -->|no| Print["<rev>"]
    PrintDirty --> End([終了])
    Print --> End
    Dev --> End
```
