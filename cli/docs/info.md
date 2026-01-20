# `esb info` コマンド (デフォルト)

## 概要

`esb info` コマンドは、CLIバージョン、設定パス、アクティブなプロジェクトの詳細、および環境のランタイム状態を含む、現在のシステム状態のサマリーを表示します。

**注意**: 引数なしで `esb` を実行した場合、このコマンドが暗黙的に実行されます。

## 使用方法

```bash
esb
# または
esb info
```

## 実装詳細

コマンドのロジックは `cli/internal/commands/info.go` に実装されています。`DetectorFactory` が利用可能な場合のみ `StateDetector` を生成し、Docker/ファイルシステムを照会して状態を判定します。軽量コマンドでは Docker 初期化を省略するため、状態が `unknown` になることがあります。

### 表示情報

1. **Version**: CLIのバージョン。
2. **Config**: グローバル設定のパス。
3. **Project**:
   - 名前とルートディレクトリ。
   - ジェネレーター設定パス (`generator.yml`)。
   - SAMテンプレートパス。
   - 出力ディレクトリ。
4. **Environment**:
   - アクティブな環境名とモード (例: `local (docker)`)。
   - **State**: `StateDetector` を介して導出された状態 (例: `running`, `stopped`, `built`)。Docker 未初期化時は `unknown`。
   - Composeプロジェクト名 (`esb-local`)。

### ロジックフロー

1. **グローバル設定読み込み**: `~/.esb/config.yaml` を検証します（`ESB_CONFIG_PATH`/`ESB_CONFIG_HOME` があれば優先）。
2. **プロジェクト解決**: アクティブなプロジェクトを特定します。
3. **状態検出**:
   - `DetectorFactory` がある場合にのみ `StateDetector` を構築し、Dockerおよびファイルシステムをクエリします。
   - Docker 初期化が省略された場合は `unknown` を表示します。

## フローチャート

```mermaid
flowchart TD
    Start([esb info]) --> LoadGlobal[グローバル設定読み込み]
    LoadGlobal --> ResolveProj[プロジェクト解決]
    ResolveProj --> ResolveCtx[コンテキスト解決]

    ResolveCtx --> DetectState[StateDetector.Detect]
    DetectState --> QueryDocker[Docker状態クエリ]
    DetectState --> CheckFS[成果物チェック]

    QueryDocker --> Result[状態決定]
    CheckFS --> Result

    Result --> Display[情報表示]
```
