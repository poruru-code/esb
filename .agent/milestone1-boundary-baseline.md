# Milestone 1 Boundary Baseline (Freeze Artifact)

Last updated: 2026-02-19 13:46Z

## Purpose

この文書は、Boundary Separation Master Plan の Milestone 1 で確定した「現状の責務棚卸し」と「次マイルストーンの実装入力」を固定するための成果物です。ここに書かれた分類・移設方針を Milestone 2/3 の正本とし、個別実装時の再判断を禁止します。

## Scope and Method

調査対象は `pkg/artifactcore`、`pkg/yamlshape`、`tools/artifactctl/cmd/artifactctl`、`cli/internal/**`、`e2e/runner/**` です。

以下のコマンドで一次データを取得しました。

    rg -n "os/exec|exec\\.Command|docker\\s+buildx|CONTAINER_REGISTRY|HOST_REGISTRY_ADDR|compose|Run\\(" pkg/artifactcore pkg/yamlshape
    rg -n "artifactcore\\.ExecuteApply|artifactcore\\.ExecuteDeploy|composeprovision\\.Execute" cli tools/artifactctl e2e
    rg -n "^(type|func|var|const) [A-Z]" pkg/artifactcore/*.go pkg/artifactcore/composeprovision/*.go pkg/yamlshape/*.go
    ./tools/ci/check_tooling_boundaries.sh

## Responsibility Inventory (Current)

### `pkg/yamlshape`

`pkg/yamlshape/shape.go` は `AsMap`、`AsSlice`、`RouteKey` のみを公開し、I/O とプロセス実行を持ちません。分類は「純粋共通ロジック」で妥当です。

### `pkg/artifactcore` の内訳

`pkg/artifactcore/manifest.go`、`pkg/artifactcore/apply.go`、`pkg/artifactcore/merge.go`、`pkg/artifactcore/merge_yaml.go`、`pkg/artifactcore/runtime_meta_validation.go`、`pkg/artifactcore/errors.go`、`pkg/artifactcore/hash.go` は、artifact 契約（manifest、validate、merge、strict 検証）に属するため「共通契約ロジック」に分類します。

`pkg/artifactcore/execute.go` は `ExecuteApply` に加えて `ExecuteDeploy` を公開し、`prepareImages` を通じて実行順序を固定しています。`ExecuteDeploy` は契約層ではなくユースケースオーケストレーション責務です。

`pkg/artifactcore/prepare_images.go` は `os/exec`、`exec.Command`、`docker buildx/tag/push`、環境変数 `CONTAINER_REGISTRY`/`HOST_REGISTRY_ADDR`、`.dockerignore` 一時書換を含みます。これは契約ロジックではなく「運用実行アダプタ責務」です。

`pkg/artifactcore/composeprovision/composeprovision.go` は artifact 固有ではない compose provision 実行ユーティリティであり、`artifactcore` 配下に置くと責務境界が誤解されます。

## Call-Site Inventory (Current)

`artifactcore.ExecuteDeploy` の実利用は `tools/artifactctl/cmd/artifactctl/main.go` のみです。`cli` と `e2e` は直接使っていません。

`artifactcore.ExecuteApply` は `cli/internal/command/artifact.go` と `cli/internal/usecase/deploy/deploy_runtime_provision.go` から利用されています。これは「artifact適用契約の共通化」として妥当です。

`composeprovision.Execute` は `tools/artifactctl/cmd/artifactctl/main.go` と `cli/internal/infra/deploy/compose_provisioner.go` の両方から使われています。再利用実体はあるが、所属パッケージが不適切です。

## Boundary Violations and Gaps

現在の `tools/ci/check_tooling_boundaries.sh` は import 方向と `go.mod` replace を検査しますが、`pkg/artifactcore` 内の禁止責務（`os/exec` や Docker 実行）を検査しません。このため「ルールはあるが検知できない」状態です。

つまり、現状は「明示的な逆依存違反は検出されない」が、「責務過多」は検出できない設計ギャップが残っています。

## Freeze Decisions for Milestone 2/3

### Decide: Keep in `pkg/artifactcore` (Contract Core)

以下は `pkg/artifactcore` に維持します。

- Manifest schema/ID/path validation (`manifest.go`)
- Runtime metadata strict validation (`runtime_meta_validation.go`)
- Required secret validation (`apply.go`)
- Runtime-config merge semantics (`merge.go`, `merge_yaml.go`, `merge_io.go`)
- Shared contract errors/hash (`errors.go`, `hash.go`)
- `ExecuteApply` entrypoint（名称維持 or `Apply` へ改名は Milestone 2 で判断）

### Decide: Move out of `pkg/artifactcore`

以下は `pkg/artifactcore` から排出します。

- `ExecuteDeploy` / `DeployInput`（`execute.go`）: destination は `tools/artifactctl` の command/usecase 層。
- `prepare_images.go` 全体: destination は `tools/artifactctl` 側の deploy operation パッケージ（候補: `tools/artifactctl/pkg/deployops`）。

### Decide: Re-home shared compose provision utility

`pkg/artifactcore/composeprovision` は `artifactcore` から分離します。移設先は Milestone 3 で確定しますが、第一候補は artifact 非依存の新規共有パッケージ（候補: `pkg/composeprovision`）です。条件は「artifact 契約への依存を持たないこと」です。

## API Freeze (for next milestone)

Milestone 2 の開始時点で、`pkg/artifactcore` の公開APIは次の3グループのみを許容対象とします。

- artifact manifest contract API
- artifact apply/merge contract API
- contract-level errors/types

`DeployInput` / `ExecuteDeploy` / `CommandRunner` は縮小対象として凍結します。

## Milestone 1 Completion Check

以下の4条件で凍結判定を行い、すべて満たしたため Milestone 1 は完了とします。

1. 未分類ファイルがない。
2. 移設対象に destination 候補が付与されている。
3. 境界違反の検知漏れ（責務過多）が明示化されている。
4. Milestone 2/3 の入力（keep/move/API縮小）が固定されている。

## Next Inputs

Milestone 2 は以下を直接入力にします。

- `pkg/artifactcore/execute.go` から `ExecuteDeploy`/`DeployInput` を削除。
- `pkg/artifactcore/prepare_images.go` の移設。
- `tools/artifactctl/cmd/artifactctl/main.go` の呼び出し更新。

Milestone 3 は以下を直接入力にします。

- `pkg/artifactcore/composeprovision` の移設と import 更新。
