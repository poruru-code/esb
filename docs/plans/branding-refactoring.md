# ブランディングツール対応計画

> **Status**: Phase 1 完了 (ESB側)、Phase 2 未着手 (Tool側)

## 概要

`esb-gemini` リポジトリにおける Phase 1 のリファクタリングが完了しました。
本ドキュメントは `esb-branding-tool` が対応すべき残タスクを定義します。

---

## Phase 2: Tool 側の実装タスク

### Task 2-1: `config/defaults.env` 生成ロジック

**対象**: `esb-branding-tool`

```python
# tools/branding/generate.py

def generate_defaults_env(branding: Branding, output_dir: Path):
    """config/defaults.env を生成する"""
    content = f"""# Project defaults - Core branding values
# This file is managed by the branding tool.

CLI_CMD={branding.cli_name}
IMAGE_PREFIX={branding.slug}
ENV_PREFIX={branding.env_prefix}
PROJECT_NAME={branding.slug}
"""
    (output_dir / "config" / "defaults.env").write_text(content)
```

### Task 2-2: `meta/meta.go` 生成ロジック

**対象**: `esb-branding-tool`

```python
def generate_meta_go(branding: Branding, output_dir: Path):
    """meta/meta.go を生成する"""
    content = f'''package meta

const (
    AppName     = "{branding.cli_name}"
    Slug        = "{branding.slug}"
    EnvPrefix   = "{branding.env_prefix}"
    EnvVarEnv   = "ENV"
    ImagePrefix = "{branding.slug}"
    LabelPrefix = "com.{branding.slug}"
    HomeDir     = ".{branding.slug}"
    OutputDir   = ".{branding.slug}"
    StagingDir  = ".staging"
    
    // Certificate Constants
    RootCAMountID      = "{branding.slug}_root_ca"
    RootCACertFilename = "rootCA.crt"
    RootCACertPath     = "/usr/local/share/ca-certificates/rootCA.crt"
    
    // Runtime / CNI Constants
    RuntimeContainerPrefix = Slug
    RuntimeNamespace       = Slug
    RuntimeCNIName         = Slug + "-net"
    RuntimeCNIBridge       = Slug + "0"
    RuntimeCNIDir          = "/run/" + Slug + "/cni"
    RuntimeResolvConfPath  = "/run/containerd/" + Slug + "/resolv.conf"
    RuntimeLabelEnv        = Slug + "_env"
    RuntimeLabelFunction   = Slug + "_function"
    RuntimeLabelCreatedBy      = "created_by"
    RuntimeLabelCreatedByValue = Slug + "-agent"
)
'''
    (output_dir / "meta" / "meta.go").write_text(content)
```

### Task 2-3: `--check` ロジックの更新

**対象**: `esb-branding-tool`

整合性チェックを2ファイルのみに簡略化:

| 対象ファイル | 検証内容 |
|:---|:---|
| `config/defaults.env` | キーと値の完全一致 |
| `meta/meta.go` | 生成コードとの完全一致 |

---

## Phase 4: テンプレート削除 (Tool側)

### Task 4-1: 削除対象テンプレート

以下のテンプレートは不要になりました:

| カテゴリ | テンプレート | 理由 |
|:---|:---|:---|
| Dockerfile | `*.Dockerfile.tmpl` (5) | ARG + defaults.env で動的化済み |
| Docker Compose | `docker-compose*.yml.tmpl` (6) | env_file で動的化済み |
| CNI Config | `10-cni.conflist.tmpl` | Agent で動的生成 |
| Shell Scripts | `entrypoint.sh.tmpl` 等 (3) | 環境変数参照に移行 |
| Go Generated | `*_branding_gen.go.tmpl` (2) | `meta/meta.go` に統合 |

**削減数: 16 → 2 ファイル**

---

## 生成対象ファイル (最終形)

| ファイル | 生成方法 |
|:---|:---|
| `config/defaults.env` | Task 2-1 で生成 |
| `meta/meta.go` | Task 2-2 で生成 |

---

## 検証基準

Tool 実装後、以下を確認:

1. `branding-tool --check` が正常終了
2. `branding-tool generate` 後に `uv run e2e/run_tests.py --reset --parallel` 成功
3. リブランド (例: `esb` → `myproject`) 時にコード変更不要
