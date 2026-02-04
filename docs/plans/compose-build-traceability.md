<!--
Where: docs/plans/compose-build-traceability.md
What: docker compose up --build と esb build でトレーサビリティを確保する詳細設計。
Why: provenance 未使用の前提で、ビルド由来メタデータを成果物へ焼き込む方式を規定する。
-->

# Compose/Docker build でのトレーサビリティ設計（Git 自己生成方式）

ステータス: 提案  
作成日: 2026-01-25  
オーナー: Architecture

## 1. 目的
- `docker compose up --build` と `esb build` の両方でトレーサビリティを確保する。
- SAM テンプレート由来の関数/ベースイメージも対象に含める。
- `.env` や CI を使わず、ローカル開発ビルドだけで完結させる。
- 運用入力としての `ESB_VERSION/GIT_SHA/BUILD_DATE` を排除する。

## 2. 非目的
- BuildKit provenance は使用しない。
- 実行時 API でのバージョン表示は提供しない。
- レジストリへの push は前提にしない。
- AWS SAM CLI の `sam build` は対象外とする（ESB は SAM テンプレートを直接解析する）。

## 3. 前提・制約
- BuildKit が有効であること（`docker build` / `docker compose` の双方で必須）。
- `.git` がローカルに存在すること（git クローン前提）。
- 開発/検証/E2E の起動は `docker compose up --build` を維持する（本番は 8.2 に従う）。
- `.env` にビルドメタを置かない。
- `DOCKER_BUILDKIT=0` の無効化は非対応。
- `additional_contexts` を使うため、Docker Compose プラグインは `v2.20+` を必須とする（README に明記）。
- `docker build --build-context` をサポートしていること（`docker build --help` に含まれることを確認）。

### 3.1 `.git` がファイルのケース（worktree / submodule 等）
- worktree / submodule では `.git` が **ファイル**になるため、Bake の `git_dir/git_common` に渡すパスを明示する。
- `docker buildx bake` 実行時に `--set meta.contexts.git_dir=...` /
  `--set meta.contexts.git_common=...` を指定する。
- 通常 clone では未指定でよい（既定 `.git` を使う）。
- いずれも **絶対パス**で指定する。
- `esb build` は CLI 内部で `gitdir/commondir` を解決し、ユーザー設定は不要とする。

## 4. 方針（採用案）
### 4.1 要旨
- ビルド中に Git 情報を自己生成し、**イメージ内ファイル**へ焼き込む。
- `docker compose up --build` / `esb build` いずれでも情報が確定する。
- ランタイム環境変数としての `ESB_VERSION/GIT_SHA/BUILD_DATE` を廃止する。

### 4.2 生成場所
共通のメタデータファイルとして以下に格納する。
- `/app/version.json`

### 4.3 参照方法
- ローカル: `docker create` + `docker cp` でファイル取得。
- 例: 「運用時のコマンド例」を参照。

## 5. メタデータ仕様
### 5.1 JSON スキーマ
`/app/version.json` の形式は以下とする。

```json
{
  "version": "0.0.0-dev.ab12cd34ef56",
  "git_sha": "ab12cd34ef56...",
  "git_sha_short": "ab12cd34ef56",
  "build_date": "2026-01-25T04:12:55Z",
  "repo_url": "git@github.com:org/repo.git",
  "source": "git"
}
```

### 5.2 値の定義
- `version`:
  - `git describe --tags --always` を基本とする。
  - `git describe` が失敗した場合は `0.0.0-dev.<git_sha_short>`。
- `git_sha`:
  - `git rev-parse HEAD` のフル SHA。
- `git_sha_short`:
  - `git rev-parse --short=12 HEAD`。
- `build_date`:
  - ビルド時刻（UTC, RFC3339）。
- `repo_url`:
  - `git config --get remote.origin.url` があれば使用。
  - `userinfo` を含む URL（`https://user:token@...` 等）は破棄する。
  - 制御文字/改行を含む場合は破棄する。
  - 無い場合は空文字。
- `source`:
  - 固定で `git`。
## 6. ビルド処理詳細
### 6.1 共通アルゴリズム
1) `gitdir/commondir`（追加コンテキスト）から `git_sha` / `version` / `repo_url` を取得。  
2) `date -u` で `build_date` を生成。  
3) `/app/version.json` に JSON を出力。  

### 6.1.1 共通スクリプト
メタデータ生成ロジックは **単一のスクリプトに集約**する。

- 配置: `tools/traceability/generate_version_json.py`
- 先頭に `Where/What/Why` のヘッダーコメントを付与する。
- 依存: Python 3 標準ライブラリのみ（追加パッケージ不要）
- 入力（必須）:
  - `--git-dir` / `--git-common-dir`
  - `--output`

- 出力:
  - `--output` で指定した JSON ファイル（UTF-8, `ensure_ascii=True`）
- 挙動:
  - `GIT_DIR/GIT_COMMON_DIR` を指定して `git` を実行する。
  - `git describe --tags --always` が失敗した場合は `0.0.0-dev.<shortsha>`。
  - `repo_url` は制御文字・改行を除外し、`https://user:token@` 等の `userinfo` を破棄する。
  - 必須引数が欠けている場合は非 0 で終了する。

### 6.2 メタデータ生成の集約（Bake）
全コンポーネントの Dockerfile から **メタデータ生成ステージ**を削除し、`version.json` は Docker Bake で **1 回だけ生成**して全イメージへ配布する。

例: `tools/traceability/Dockerfile.meta` を Bake で実行し、`output=type=local` で `version.json` を出力する。

### 6.3 最終ステージへのコピー
最終ステージに以下を追加する。

```Dockerfile
COPY --from=meta /version.json /app/version.json
```

### 6.4 既存ビルドメタの整理
- Dockerfile 内の `build-meta` ステージを廃止する。
- runtime 系のみ `IMAGE_RUNTIME` を `ENV` に焼き込む（entrypoint が参照）。
- base / function 系は `ENV` に焼き込まない（不要な環境変数を増やさない）。

## 7. Compose 設定
### 7.1 追加コンテキスト
`version.json` を共有するために `meta` コンテキストを追加する。
Compose ファイルは **branding ツール（esb-branding-tool）で生成**されるため、
修正はツール側テンプレートで行い、生成物（`docker-compose.*.yml`）へ反映する。

- `META_CONTEXT` 未設定時は `.<brand>/meta` を使用する。
- `docker compose up --build` を使う場合、事前に `version.json` を生成しておく。
  - 例: `docker buildx bake -f tools/traceability/docker-bake.hcl meta`
  - `.git` がファイルのケースは `--set meta.contexts.git_dir=...` /
    `--set meta.contexts.git_common=...` を追加する（詳細は 3.1）。
- agent のビルドでは Go module `meta` を参照するため、`meta_module` を追加する。
  - `META_MODULE_CONTEXT` 未設定時は `meta` を使用する。

例: `docker-compose.docker.yml`（gateway の場合）

```yaml
services:
  gateway:
    build:
      context: services/gateway
      dockerfile: Dockerfile.docker
      additional_contexts:
        config: ${CONFIG_DIR:-services/gateway/config}
        meta: ${META_CONTEXT:-.<brand>/meta}
        common: ${COMMON_CONTEXT:-services/common}
  agent:
    build:
      context: services/agent
      dockerfile: Dockerfile.docker
      additional_contexts:
        meta: ${META_CONTEXT:-.<brand>/meta}
        meta_module: ${META_MODULE_CONTEXT:-meta}
```

### 7.1.1 必須 build args
- Root CA 関連の build args は既存どおり。
- `IMAGE_RUNTIME` は Dockerfile の `ENV` で固定し、ビルド引数としては渡さない。

### 7.2 esb build（docker buildx bake）での追加コンテキスト
- `esb build` は関数/ベースイメージのビルドに `docker buildx bake` を使用する。
- CLI は build 前に Docker Bake で `version.json` を 1 回生成し、`.<brand>/meta` に出力する。
- `tools/traceability/docker-bake.hcl` に base ターゲットを追加し、実行時に生成する bake ファイルで
  function ターゲットと build args/labels/contexts を上書きする。
- 各 bake ターゲットに `contexts.meta` を追加し、`version.json` を共有する。
- `gitdir/commondir` は Bake 用に解決し、worktree でも CLI が自動で処理する。

#### 7.2.1 CLI 実装詳細（コードレベル）
- 変更対象:
  - `cli/internal/generator/go_builder.go`
  - `cli/internal/generator/go_builder_helpers.go`
- `Build()` の先頭で `gitdir/commondir` を解決し、Bake 用コンテキストとして渡す。
- `resolveGitContext` の呼び出しは `b.Runner` を使用する（本番は `compose.ExecRunner`）。
- Bake が失敗した場合は `esb build` を即時失敗させ、エラーメッセージに `git rev-parse` の失敗理由を含める。
- `prepareMetaContext` で `docker buildx bake` を実行し、`.<brand>/meta/version.json` を生成する。
- Bake 用の補助関数を追加し、base/functions を並列ビルドする。

```go
type buildContext struct {
	Name string
	Path string
}

func buildDockerImage(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	dockerfile string,
	imageTag string,
	noCache bool,
	verbose bool,
	labels map[string]string,
	buildContexts []buildContext,
) error
```

- 追加対象の bake 実行:
  - base イメージ（lambda-base / os-base / python-base）
  - `buildFunctionImages()`（各関数イメージ）
- `buildContexts` には `meta` のみを入れる。
- `trace_tools` の実体は `filepath.Join(repoRoot, "tools", "traceability")` とし、
  `generate_version_json.py` の存在を確認してから Bake に渡す。

#### 7.2.2 gitdir/commondir 解決ロジック
新規ヘルパーを追加し、`compose.ExecRunner`（内部で `exec.Command` を使用）で解決する。
この値は `docker buildx bake` のコンテキスト解決にのみ使用する。

- 追加ファイル: `cli/internal/generator/git_context.go`
  - 先頭に `Where/What/Why` のヘッダーコメントを付与する。
- I/F 例:

```go
type gitContext struct {
	GitDir    string
	GitCommon string
}

type gitRunner interface {
	RunOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

func resolveGitContext(ctx context.Context, runner gitRunner, repoRoot string) (gitContext, error)
```

実装仕様（擬似コード）:

```go
func resolveGitContext(ctx context.Context, runner gitRunner, repoRoot string) (gitContext, error) {
	root := filepath.Clean(strings.TrimSpace(repoRoot))
	if root == "" {
		return gitContext{}, fmt.Errorf("repo root is required")
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return gitContext{}, fmt.Errorf("repo root resolve failed: %w", err)
	}
	top, err := runGit(ctx, runner, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return gitContext{}, err
	}
	topResolved, err := filepath.EvalSymlinks(top)
	if err != nil {
		return gitContext{}, fmt.Errorf("git top resolve failed: %w", err)
	}
	if filepath.Clean(topResolved) != filepath.Clean(rootResolved) {
		return gitContext{}, fmt.Errorf("repo root mismatch: %s", top)
	}
	gitDirRaw, err := runGit(ctx, runner, root, "rev-parse", "--git-dir")
	if err != nil {
		return gitContext{}, err
	}
	gitCommonRaw, err := runGit(ctx, runner, root, "rev-parse", "--git-common-dir")
	if err != nil {
		return gitContext{}, err
	}
	gitDir, gitDirIsFile, err := resolveGitDir(root, gitDirRaw)
	if err != nil {
		return gitContext{}, err
	}
	gitCommon, err := resolveGitCommon(root, gitDir, gitDirIsFile, gitCommonRaw)
	if err != nil {
		return gitContext{}, err
	}
	if _, err := os.Stat(filepath.Join(gitDir, "HEAD")); err != nil {
		return gitContext{}, fmt.Errorf("gitdir missing HEAD: %w", err)
	}
	if _, err := os.Stat(filepath.Join(gitCommon, "objects")); err != nil {
		return gitContext{}, fmt.Errorf("git common dir missing objects: %w", err)
	}
	return gitContext{GitDir: gitDir, GitCommon: gitCommon}, nil
}

func runGit(ctx context.Context, runner gitRunner, root string, args ...string) (string, error) {
	out, err := runner.RunOutput(ctx, root, "git", args...)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, msg)
	}
	val := strings.TrimSpace(string(out))
	if val == "" {
		return "", fmt.Errorf("git %s returned empty output", strings.Join(args, " "))
	}
	return val, nil
}

func resolveGitDir(root, gitDirRaw string) (string, bool, error) {
	gitDirPath := resolveAbs(root, gitDirRaw)
	info, err := os.Stat(gitDirPath)
	if err != nil {
		return "", false, fmt.Errorf("gitdir not found: %w", err)
	}
	if info.IsDir() {
		return gitDirPath, false, nil
	}
	content, err := os.ReadFile(gitDirPath)
	if err != nil {
		return "", false, fmt.Errorf("gitdir read failed: %w", err)
	}
	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", false, fmt.Errorf("gitdir file format invalid")
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
	if target == "" {
		return "", false, fmt.Errorf("gitdir file is empty")
	}
	return resolveAbs(filepath.Dir(gitDirPath), target), true, nil
}

func resolveGitCommon(root, gitDir string, gitDirIsFile bool, gitCommonRaw string) (string, error) {
	base := root
	if gitDirIsFile {
		base = gitDir
	}
	return resolveAbs(base, gitCommonRaw), nil
}

func resolveAbs(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}
```

解決手順:
1) `git rev-parse --show-toplevel` を `repoRoot` で実行し、`EvalSymlinks` で正規化したパス同士を比較する。  
2) `git rev-parse --git-dir` と `git rev-parse --git-common-dir` を同一の `repoRoot` で実行。  
3) `gitdir` が **ファイル**なら `gitdir: <path>` を読み取り、実体パスへ変換。  
4) 相対パスは `gitdir` がファイルの場合は **gitdir 実体**起点、通常は `repoRoot` 起点で絶対パス化する。  
5) `GitDir/HEAD` の存在を確認し、無ければエラー。  
6) `GitCommon/objects` の存在を確認し、無ければエラー。  

#### 7.2.3 `docker build` 引数の構築
- `buildDockerImage()` 内で `--build-context` を追加する。
- build arg/label/secret と併用し、`"."` の直前に付与する。
- 例:

```bash
docker build \
  --build-context meta=/abs/path/to/.<brand>/meta \
  -f <Dockerfile> -t <tag> .
```

#### 7.2.4 テスト
- `cli/internal/generator/go_builder_test.go`:
  - `docker buildx bake` の `--set meta.contexts.*` と
    `meta.output=type=local,dest=...` が含まれることを検証。
  - `docker build` の引数に `--build-context meta=...` が含まれることを検証。
  - worktree 相当の gitdir ファイルを使った `resolveGitContext` のユニットテストを追加。
  - 追加テストケース（`cli/internal/generator/git_context_test.go`）:
    - 先頭に `Where/What/Why` のヘッダーコメントを付与する。
    - `TestResolveGitContext_StandardRepo`:
      - `.git/HEAD` と `.git/objects` を持つ構成で解決できること。
      - `git rev-parse --git-dir` = `.git`, `--git-common-dir` = `.git` を想定。
    - `TestResolveGitContext_WorktreeGitDirFile`:
      - `.git` が **ファイル**で `gitdir: ../.git/worktrees/w1` の形式でも解決できること。
      - `git-common-dir` が `../.git` の相対パスであっても絶対パス化されること。
    - `TestResolveGitContext_MissingHead`:
      - `gitdir/HEAD` が無い場合にエラーとなること。
    - `TestResolveGitContext_MissingObjects`:
      - `gitcommon/objects` が無い場合にエラーとなること。

### 7.3 既存変数の廃止
以下を Compose から削除する。
- `ESB_VERSION`
- `GIT_SHA`
- `BUILD_DATE`

## 8. 運用時のコマンド例
### 8.0 目次
- 8.1 開発/検証 起動
- 8.2 本番リリース運用（タグ付与/起動）
- 8.3 worktree 使用時の前準備（compose 手動実行のみ）
- 8.4 esb build
- 8.5 手動 docker build（関数/ベースイメージ）
- 8.6 メタデータ取得（ローカル）
- 8.7 主要項目の簡易確認
- 8.8 注意（imagetools inspect）

### 8.1 開発/検証 起動
```bash
docker compose up --build
```
※ containerd モードで起動する場合は `<BRAND>_REGISTRY` の指定が必須。

### 8.2 本番リリース運用（タグ付与/起動）
#### 8.2.1 タグ付与フロー
1) リリース対象コミットに移動し、タグを作成する。
```bash
git tag -a vX.Y.Z -m "release vX.Y.Z"
git push origin vX.Y.Z
```
2) タグ付きコミットからビルドする（`git describe --tags --always` が `vX.Y.Z` になる）。

#### 8.2.2 本番起動（Docker）
```bash
export <BRAND>_TAG=vX.Y.Z
esb build --template template.yaml --env prod --mode docker
docker compose -f docker-compose.docker.yml --env-file .env.prod up -d
```
※ runtime 系のタグは `<BRAND>_TAG` に `-docker` を付与したものを使用する。

#### 8.2.3 本番起動（containerd / firecracker）
```bash
export <BRAND>_TAG=vX.Y.Z
export <BRAND>_REGISTRY=registry.example.com/
esb build --template template.yaml --env prod --mode containerd
CONTAINERD_RUNTIME=aws.firecracker docker compose -f docker-compose.containerd.yml --env-file .env.prod up -d
```
※ containerd 系は `<BRAND>_REGISTRY` が必須。
※ runtime 系のタグは `<BRAND>_TAG` に `-containerd` を付与したものを使用する。

#### 8.2.4 トレーサビリティ確認
```bash
image="<brand>-gateway:vX.Y.Z-docker"
cid=$(docker create "$image")
docker cp "$cid:/app/version.json" ./version.json
docker rm "$cid"
cat ./version.json
```

### 8.3 `.git` がファイルのケースの前準備（worktree / submodule / compose 手動実行のみ）
`docker buildx bake` で meta を生成する際に `git_dir` / `git_common` を指定する。

```bash
root="$(git rev-parse --show-toplevel)"
gitdir="$(git rev-parse --git-dir)"
commondir="$(git rev-parse --git-common-dir)"
resolve() { case "$1" in /*) echo "$1" ;; *) echo "$2/$1" ;; esac; }
if [ -f "${gitdir}" ]; then
  base_dir="$(dirname "${gitdir}")"
  gitdir="$(sed -n 's/^gitdir: //p' "${gitdir}")"
  gitdir="$(resolve "$gitdir" "$base_dir")"
  common_base="${gitdir}"
else
  gitdir="$(resolve "$gitdir" "$root")"
  common_base="${root}"
fi
commondir="$(resolve "$commondir" "$common_base")"
docker buildx bake -f tools/traceability/docker-bake.hcl meta \
  --set meta.contexts.git_dir="${gitdir}" \
  --set meta.contexts.git_common="${commondir}" \
  --set meta.contexts.trace_tools="${root}/tools/traceability" \
  --set meta.output=type=local,dest="${root}/.<brand>/meta"
docker compose up --build
```

### 8.4 esb build
```bash
esb build --template ./template.yaml --env dev --mode docker
```
※ `esb build` は CLI 内部で Bake を実行し、`meta` を生成してから `docker build` を実行する。

### 8.5 手動 docker build（関数/ベースイメージ）
```bash
root="$(git rev-parse --show-toplevel)"
docker buildx bake -f tools/traceability/docker-bake.hcl meta \
  --set meta.contexts.git_dir="${root}/.git" \
  --set meta.contexts.git_common="${root}/.git" \
  --set meta.contexts.trace_tools="${root}/tools/traceability" \
  --set meta.output=type=local,dest="${root}/.<brand>/meta"
docker build \
  --build-context meta="${root}/.<brand>/meta" \
  -f ./Dockerfile.lambda \
  -t "<brand>-fn:dev" \
  .
```

### 8.6 メタデータ取得（ローカル）
```bash
image_name="<brand>-gateway:latest-docker"
container_id=$(docker create "${image_name}")
docker cp "${container_id}:/app/version.json" ./version.json
docker rm "${container_id}"
cat ./version.json
```

### 8.7 主要項目の簡易確認
```bash
cat ./version.json | jq -r '.version,.git_sha,.build_date'
```
※ `jq` が無い場合は `cat` のみでも確認可能。

### 8.8 注意（imagetools inspect）
- 本方式は registry への push を前提にしないため、`docker buildx imagetools inspect` は利用しない。
- トレーサビリティの確認は `/app/version.json` を参照する。

## 9. 失敗時の挙動
- Bake 実行時に `gitdir/commondir` が解決できない場合:
  - `git rev-parse` が失敗して `esb build` が中断。
- `git describe` が失敗:
  - `version` を `0.0.0-dev.<shortsha>` にフォールバック。
- `docker buildx bake` が失敗:
  - `version.json` が生成できず `esb build` は失敗。
- `docker compose up --build` で `META_CONTEXT` が未生成の場合:
  - `meta` コンテキストが無いためビルド失敗。
- `trace_tools` が見つからない場合:
  - Bake 用スクリプトが実行できずビルド失敗。

## 9.1 備考（dirty 判定）
- 作業ツリーをマウントしないため `dirty` 判定は行わない。
- `dirty` が必要な場合は別途、作業ツリーを含むコンテキストを追加する。

## 10. 検証項目
- すべてのコンポーネントで `/app/version.json` が生成されること。
- `git rev-parse HEAD` と `version.json.git_sha` が一致すること。
- `version.json` が Bake で 1 回のみ生成されていること（ログで確認）。
- `esb build` による関数/ベースイメージでも `version.json` が生成されること。

## 11. 影響範囲（変更対象）
- Dockerfiles:
  - `services/common/Dockerfile.os-base`
  - `services/common/Dockerfile.python-base`
  - `services/gateway/Dockerfile.docker`
  - `services/gateway/Dockerfile.containerd`
  - `services/agent/Dockerfile.docker`
  - `services/agent/Dockerfile.containerd`
  - `services/provisioner/Dockerfile.docker`
  - `services/provisioner/Dockerfile.containerd`
  - `services/runtime-node/Dockerfile.containerd`
  - `cli/internal/generator/assets/Dockerfile.lambda-base`
  - `cli/internal/generator/templates/dockerfile.tmpl`
- Compose（生成物）:
  - `docker-compose.docker.yml`
  - `docker-compose.containerd.yml`
  - 生成元は外部の branding ツール（`esb-branding-tool`）の
    `tools/branding/templates/docker-compose.*.yml.tmpl` を更新する。
- CLI:
  - `cli/internal/generator/go_builder.go`
  - `cli/internal/generator/go_builder_helpers.go`
  - `cli/internal/generator/git_context.go`
  - `cli/internal/generator/git_context_test.go`
  - `cli/internal/generator/go_builder_test.go`
  - `cli/internal/helpers/env_defaults.go`
- Traceability script:
  - `tools/traceability/generate_version_json.py`
- Docs:
  - `docs/plans/runtime-image-architecture.md`

## 12. ロールバック
- `ESB_VERSION/GIT_SHA/BUILD_DATE` の `ARG/ENV` を元の Dockerfile に戻す。
- Compose から削除した変数を復帰させる。
