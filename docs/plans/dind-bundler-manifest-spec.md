<!--
Where: docs/plans/dind-bundler-manifest-spec.md
What: DinD bundler manifest-driven design spec.
Why: Make bundle contents deterministic and template-derived; prevent host-state contamination.
-->
# DinD Bundler: Manifest-Driven Bundle Specification

この仕様の JSON Schema は `docs/plans/dind-bundler-manifest.schema.json` に定義する。

## 背景と課題
現在の DinD バンドラーは、ホスト上の Docker に存在する `com.<brand>.kind=function` ラベル付きイメージを探索して取り込む。これはテンプレート由来の成果物だけをパッケージする設計要件と矛盾し、以下の問題を引き起こす。

- テンプレートに無関係な過去ビルドの関数イメージが混入する
- 再現性がホスト状態に依存し、設計上の入力境界が曖昧になる
- 監査・検証可能性が低く、由来の証明ができない

本ドキュメントは、**「テンプレートから一意に導出される内容のみをバンドルする」**ための、あるべき設計仕様を定義する。

## あるべき姿（非交渉条件）

- **決定性**: バンドル内容はテンプレートとパラメータから一意に導出される
- **由来の証明**: どのテンプレート・どのパラメータ・どの成果物から生まれたか追跡可能
- **境界の厳密性**: ホスト上の Docker 状態は入力に含めない
- **再現性**: 同一テンプレートとパラメータで同一バンドルを生成できる

これらを満たすため、**マニフェストを唯一の真実（source of truth）**とし、バンドラーは探索ではなくマニフェストに従って動作する。

---

# 仕様

## 1. マニフェストの役割
マニフェストは、テンプレートから導出される**全てのバンドル対象イメージの宣言的リスト**である。
バンドラーはこれを唯一の入力とし、ホスト状態の探索を禁止する。

## 2. マニフェスト項目（提案仕様）

### 2.1 形式
- JSON を基本形式とする（機械可読・検証容易）
- ファイル名: `bundle/manifest.json`（生成物として同梱）

### 2.2 主要フィールド

```json
{
  "schema_version": "1.0",
  "generated_at": "2026-01-24T13:29:51Z",
  "template": {
    "path": "e2e/fixtures/template.yaml",
    "sha256": "<template file hash>",
    "parameters": {
      "KeyA": "ValueA"
    }
  },
  "build": {
    "project": "esb-default",
    "env": "default",
    "mode": "docker",
    "image_prefix": "esb",
    "image_tag": "docker",
    "git": {
      "commit": "<git sha>",
      "dirty": false
    }
  },
  "images": [
    {
      "name": "esb-hello:docker",
      "digest": "sha256:...",
      "kind": "function",
      "source": "template",
      "labels": {
        "com.<brand>.project": "<brand>-default",
        "com.<brand>.env": "default",
        "com.<brand>.kind": "function"
      },
      "platform": "linux/amd64"
    },
    {
      "name": "esb-gateway:docker",
      "digest": "sha256:...",
      "kind": "service",
      "source": "internal",
      "platform": "linux/amd64"
    },
    {
      "name": "scylladb/scylla@sha256:...",
      "digest": "sha256:...",
      "kind": "external",
      "source": "external",
      "platform": "linux/amd64"
    }
  ]
}
```

### 2.3 フィールド定義

- `schema_version`: マニフェスト仕様のバージョン
- `generated_at`: 生成時刻（RFC3339 UTC）
- `template.path`: テンプレートの相対パス
- `template.sha256`: テンプレートファイルのハッシュ
- `template.parameters`: 実際に使用されたパラメータ
- `build.project`: compose project 名
- `build.env`: 環境名
- `build.mode`: docker/containerd/firecracker
- `build.image_prefix`: ESB イメージ接頭辞
- `build.image_tag`: ESB イメージタグ
- `build.git`: ビルド時のコミットと dirty フラグ
- `images[]`: バンドル対象のイメージ一覧
  - `name`: 完全なイメージ参照（タグまたは digest）
  - `digest`: 取得済みイメージの digest（必須）
  - `kind`: function/service/base/external 等
  - `source`: template/internal/external 等
  - `labels`: 任意（補助情報。探索条件には使用しない）
  - `platform`: 例 `linux/amd64`

---

# 生成タイミング

## 3. マニフェスト生成

- **生成タイミング**: `esb build` の成功後、イメージ群が確定した時点
- **生成場所**: `bundle/manifest.json` を output dir に出力
- **生成責務**: Generator/Builder が、テンプレート由来の関数と内部サービス・外部依存の一覧を確定して出力

**重要**: マニフェスト生成は “探索ベース” ではなく、**テンプレートから導出**すること。

---

# 検証フロー

## 4. バンドラーの検証フロー（必須）

1) マニフェストを読み込み、全イメージの一覧を取得
2) 各イメージについて `docker image inspect` を実行
3) 取得した digest がマニフェストの `digest` と一致することを確認
4) 一致しない場合は **即失敗**
5) すべて検証できたら `docker save` により tar を作成

**禁止**: `docker image ls` で探索した結果を追加対象にすること

---

# 運用ポリシー（設計上の必須条件）

- **タグではなく digest 固定を原則とする**
  - 例: `scylladb/scylla@sha256:...`
- **テンプレート外イメージの取り込みは禁止**
- **ラベル探索は補助用途のみ**（決定ロジックに使わない）
- **マニフェスト不在の場合はビルド失敗とする**

---

# 期待される効果

- バンドル内容の完全な決定性
- テンプレートと成果物の 1:1 対応
- セキュリティ監査や検証の容易化
- 再現性と運用責任境界の明確化

---

# まとめ

DinD バンドルの“あるべき姿”は、**テンプレート由来のマニフェストのみを信頼し、それに従ってバンドルを生成すること**である。ホストの Docker 状態を探索して補う設計は、再現性と由来保証を破壊するためアーキテクチャとして不適切である。

本仕様は、それを完全に排除し、**宣言的・決定的・監査可能**なバンドル生成を実現するための基盤である。
