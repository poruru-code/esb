<!--
Where: docs/sbom-cyclonedx.md
What: CycloneDX SBOM generation and maintenance policy.
Why: Define automated SBOM operations with clear scope and ownership.
-->
# CycloneDX SBOM 運用

このドキュメントは、CycloneDX SBOM を **自動生成・自動保守**する運用方針を定義します。

## 対象範囲（本サイクル）
- 対象: 本番向けの言語依存（Python / Go / Java）
- 除外: `e2e/fixtures` 配下のサンプル・テスト用プロジェクト
- 除外: コンテナイメージSBOM（次フェーズ）

## 生成対象
### Python
- 検出条件: `uv.lock` と `pyproject.toml` が同一ディレクトリに存在
- 生成コマンド: `uv export --format cyclonedx1.5 --frozen`

### Go
- 検出条件: `go.mod` が存在
- 生成コマンド: `cyclonedx-gomod mod -licenses -json -output-version 1.5`

### Java
- 対象固定: `runtime/java/build/pom.xml`
- 生成コマンド:
  `org.cyclonedx:cyclonedx-maven-plugin:<version>:makeAggregateBom`
- Maven local repository:
  `.esb/cache/m2/repository` を使用

## ローカル実行の前提
- `uv`（Python SBOM export）
- `cyclonedx-gomod`（Go SBOM）
- Java 21 + Maven（Java SBOM）

## 出力ファイル契約
SBOM出力ディレクトリ（`sbom/`）に以下を生成します。

- `python-*.cdx.json`
- `go-*.cdx.json`
- `java-runtime.cdx.json`
- `index.json`

`index.json` には生成時刻、スキーマ、成果物一覧、生成コマンドを記録します。

## スキーマ方針
- 現在は **CycloneDX 1.5** で統一
- `tools/ci/generate_cyclonedx_sbom.py` が `bomFormat=CycloneDX` と
  `specVersion=1.5` を検証

## CI / Release 連携
ワークフロー: `.github/workflows/sbom-cyclonedx.yml`

- `pull_request`, `push(main)`, `schedule`, `workflow_dispatch`, `release: published` で実行
- `actions/upload-artifact` で `cyclonedx-sbom-${sha}` を保存
- `release: published` 時のみ Release Asset として `sbom/*` を公開

## 自動メンテナンス
ツールバージョンは `tools/ci/sbom-tool-versions.env` で一元管理します。

- `CYCLONEDX_GOMOD_VERSION`
- `CYCLONEDX_MAVEN_PLUGIN_VERSION`

`renovate.json` で以下を自動更新対象にします。

- GitHub Actions
- Go modules
- Maven
- PEP 621
- SBOMツール版（regex manager）

CycloneDX関連更新は `cyclonedx-toolchain` グループとして1つのPRに集約します。

## 障害時対応
1. `generate-sbom` ジョブログで失敗したエコシステムを特定
2. `tools/ci/generate_cyclonedx_sbom.py --strict` をローカル再実行
3. `tools/ci/sbom-tool-versions.env` の固定版を戻して再試行
4. 依存レジストリ障害時は `workflow_dispatch` で再実行

## 次フェーズ
- Syft/CycloneDX でコンテナイメージSBOMを追加
- 必要に応じて Dependency-Track など外部システム連携を検討

---

## Implementation references
- `.github/workflows/sbom-cyclonedx.yml`
- `tools/ci/generate_cyclonedx_sbom.py`
- `tools/ci/sbom-tool-versions.env`
- `renovate.json`
