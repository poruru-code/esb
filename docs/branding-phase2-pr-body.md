<!--
Where: docs/branding-phase2-pr-body.md
What: PR body draft for Phase1/2 branding neutral-key migration.
Why: Keep reviewer-facing evidence consistent with implemented scope.
-->
# PR Body Draft: ESB依存削減（Phase1/2）

## Summary
- Phase1/Phase2 として、`ESB_*` 依存を中立キー（`TAG`, `REGISTRY`, `SKIP_GATEWAY_ALIGN`, `REGISTRY_WAIT`, `CLI_BIN`, `META_REUSE`, `BUILDKITD_OVERWRITE`, `BUILDX_NETWORK_MODE`, `TINYPROXY_*`）へ移行しました。
- `runtime-safe` スコープでは `ESB_*` を 0 件化しました（docs/test を除外）。
- branding 側は `base/upstream` 命名へ統一し、`generate --check` を `esb/acme/app` で成功させています。

## Scope
- Go/Python/Shell の runtime 参照を中立キーへ統一
- `esb-branding-tool` 側 lock/info と generate/check フローの整合
- E2Eフレーク修正（非同期ログ到達待ちの共通化）
- ドキュメント整理（baseline と現行運用の分離）

## Validation
1. `runtime-safe` スコープの `ESB_*` 再計測
```bash
rg -n "ESB_(TAG|REGISTRY|SKIP_GATEWAY_ALIGN|REGISTRY_WAIT|CLI|BIN|META_REUSE|BUILDKITD_OVERWRITE|BUILDX_NETWORK_MODE)" \
  --glob '!docs/**' --glob '!**/*_test.go' --glob '!**/tests/**' --glob '!e2e/**'
```
期待値: no match

2. branding check（tool repo）
```bash
python3 tools/branding/generate.py --root /home/akira/esb --brand esb --check
python3 tools/branding/generate.py --root /tmp/esb-brand-check-acme --brand acme --check
python3 tools/branding/generate.py --root /tmp/esb-brand-check-app --brand app --check
PYTHONPATH=. uvx pytest tools/branding/tests -q
```
期待値: all pass

3. e2e（esb repo）
```bash
uv run python e2e/run_tests.py --parallel
```
期待値: `e2e-docker`, `e2e-containerd` ともに pass

## Risks / Notes
- 並列E2Eで Java smoke が一時的に失敗する揺らぎがありうるため、失敗時は単体再実行で再現性を確認してください。
- proto package 名（`esb.agent.v1`）や pb 生成物の改名は本PRスコープ外です。

## Rollback
- 中立キー化差分を revert し、`ESB_*` 互換読取ロジックを復帰させることで段階的に戻せます。
