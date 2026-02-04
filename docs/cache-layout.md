# キャッシュ構成

ステータス: 実装済み（プロジェクトスコープの staging キャッシュ）

## 概要
このドキュメントは、ESB の deploy staging データのキャッシュ構成を定義します。
グローバル設定はユーザーのホームに残し、deploy のマージ結果と staging アーティファクトは
プロジェクトスコープでテンプレートの隣に配置します。

## 目的
- グローバルで再利用可能な資産は `~/.esb` に保持する。
- deploy マージ結果をプロジェクト/テンプレート単位で保存し、プロジェクト間の混入を防ぐ。
- クリーンアップをテンプレートディレクトリ内で完結できるようにする。
- 現在のハッシュ付き staging ディレクトリ名を廃止する。

## 目的外
- buildx のキャッシュ内容や仕組みは変更しない。
- TLS / WireGuard などのグローバル資産は変更しない。

## 旧挙動（参考）
- グローバル設定は `~/.esb/config.yaml` にある。
- staging キャッシュは `~/.esb/.cache/staging/<project-hash>/<env>/...` 配下にあった。
- ハッシュは compose project + env から生成され、env もサブディレクトリとして入るため、
  構成が冗長で目視しづらかった。

## 目標の挙動（仕様）
### グローバル（変更なし）
- `~/.esb/config.yaml` は最近のテンプレートとデフォルト入力を保持する。
- `~/.esb/certs` / `~/.esb/wireguard` / `~/.esb/buildkitd.toml` はグローバルのまま。

### プロジェクトスコープ（新しいデフォルト）
テンプレートのディレクトリをキャッシュルートとして使う：

```
<template_dir>/.esb/
  staging/
    <compose_project>/
      <env>/
        config/
          functions.yml
          routing.yml
          resources.yml
          .deploy.lock
        services/
        pyproject.toml
```

注記:
- `compose_project` は docker compose のプロジェクト名（PROJECT_NAME）。
- `env` はデプロイ環境（例: dev, staging）。
- すべての staging アーティファクトは `<compose_project>/<env>` 配下に置き、
  ハッシュなしで環境ごとの衝突を避ける。

## パスと内容（表）
### グローバルキャッシュ
| パス | 内容 | 目的/備考 |
| --- | --- | --- |
| `~/.esb/config.yaml` | 最近使ったテンプレートやデフォルト入力 | グローバル設定 |
| `~/.esb/certs/` | ルート CA などの証明書 | 共有資産 |
| `~/.esb/wireguard/` | WireGuard 設定/鍵 | 共有資産 |
| `~/.esb/buildkitd.toml` | buildkitd 設定 | 共有資産 |

### プロジェクトキャッシュ（テンプレート隣）
| パス | 内容 | 目的/備考 |
| --- | --- | --- |
| `<project_root>/.esb/buildx-cache/` | buildx のローカルキャッシュ | bake の cache-to/cache-from |
| `<template_dir>/.esb/staging/<compose_project>/<env>/config/functions.yml` | 関数定義 | deploy マージ結果 |
| `<template_dir>/.esb/staging/<compose_project>/<env>/config/routing.yml` | ルーティング定義 | deploy マージ結果 |
| `<template_dir>/.esb/staging/<compose_project>/<env>/config/resources.yml` | リソース定義 | deploy マージ結果 |
| `<template_dir>/.esb/staging/<compose_project>/<env>/services/` | サービス構成 | staging アーティファクト |
| `<template_dir>/.esb/staging/<compose_project>/<env>/pyproject.toml` | 依存/環境設定 | staging アーティファクト |
| `<template_dir>/.esb/staging/<compose_project>/<env>/config/.deploy.lock` | 排他ロック | 並行実行保護 |

## パス解決ルール
staging ルートは固定で `<template_dir>/.esb/staging` を使用する。
テンプレートディレクトリが書き込み不可の場合はエラーとする。

## クリーンアップ
- 1つの env を削除:
  `rm -rf <template_dir>/.esb/staging/<compose_project>/<env>`
- 1つのプロジェクトの env を全部削除:
  `rm -rf <template_dir>/.esb/staging/<compose_project>`

グローバル設定と証明書は削除対象外。

## 互換性メモ
- 現在 `~/.esb/.cache/staging` を走査している env 推論は、新しいプロジェクトスコープの
  staging ルートを走査するよう更新が必要。
- ハッシュ付きパスは廃止。既存のグローバルキャッシュはレガシーとして扱い、
  新レイアウトでは無視する。
