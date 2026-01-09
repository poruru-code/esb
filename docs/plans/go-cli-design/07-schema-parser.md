---
title: Schema-aware SAM parser
---

## 目的
- goformation の導入や fork なしに、AWS が公開する `sam.schema.json` を **信頼できる構造定義**としてパーサーを設計・維持する。
- 現在の `parser.go` がサポートできていない `Architectures`／`CompatibleArchitectures`／`RuntimeManagementConfig` などの SAM 固有フィールドを網羅し、今後の拡張を阻害しない型安全な抽出を実現する。

## 概要設計

1. **schema を管理する**
   - `cli/internal/generator/schema/sam.schema.json` に必要な定義だけ（Functions, Globals, Layers, Resources）の抜粋を入れ、リポジトリでバージョン管理。
   - 補足的に JSON Schema の `definitions` から Go struct を手動または `quicktype` で生成し、今後のフィールド追加を追跡可能にする。

2. **パーサーの流れ**
   - YAML ファイルを読んで `intrinsics.ProcessYAML` で置換済み JSON を得る（既存と共通）。
   - `github.com/xeipuuv/gojsonschema` などで schema に対してバリデーションし、未知のプロパティや型のズレを即検出。
   - バリデーション済み JSON を `schema.Template`（`Globals`, `Resources`, `Parameters` などを含む struct）に `encoding/json` でデコード。
   - `SchemaTemplate` から `FunctionSpec`, `ResourcesSpec` を組み立て直す `TranslateSchemaTemplate` 関数で既存 API に橋渡し。

3. **型の補完**
   - `FunctionSpec` に `Architectures []string`, `RuntimeManagementConfig`、`CompatibleArchitectures` などを追加し、schema の `properties` に合わせる。
   - `LayerSpec` も `CompatibleArchitectures` を保持し、`Resolver` で `!Ref` を正しく展開するヘルパーを継続。

4. **テスト**
   - `.tmp/template.yml` を schema 準拠の fixture として保持し、`ParseSAMTemplate`（または新関数）に読み込ませ、`Architectures`, `RuntimeManagementConfig` などの結果を検証。
   - schema バリデーションが失敗した場合にも明確なエラーメッセージとテストを用意（ex: `CompatibleArchitectures` に未定義値を入れると schema で弾かれる）。

5. **メンテナンス**
   - schema 側は必要に応じて `sam.schema.json` を最新版で差し替え（定義が増えたら `resources` ディレクトリを更新）。
   - JSON Schema から struct を生成したい場合は `tools/schema/gen.go` などで `go generate` が使えるようにし、差分で型を補完。

## 今後の作業
1. schema 抜粋ファイルの追加と `gojsonschema` 依存化。
2. `SchemaTemplate` struct を実装し、`generator.ParseSAMTemplate` の上流としてバリデーション→変換を組み込む。
3. `.tmp/template.yml` などを使った TDD で `Architectures`/`RuntimeManagementConfig` の extraction を検証。
4. 必要に応じて schema 生成スクリプトを追加し、定義済みフィールドの扱いを自動化。

このアプローチなら「標準 schema をベースにしたパーサー」「ローカルで拡張できる構造」「将来のフィールド追加に耐える設計」が両立できます。次は具体的に schema struct の実装とテスト追加に着手します。
