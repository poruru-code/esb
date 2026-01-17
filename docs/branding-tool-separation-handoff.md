<!--
Where: docs/branding-tool-separation-handoff.md
What: Meeting notes + handoff for branding tool separation work.
Why: Preserve decisions, constraints, and next steps for the new session.
-->
# ブランディングツール分離: 打ち合わせメモ兼引継ぎ

## 目的
- ブランディング変更ツールを ESB 本体から分離し、下流向けのカスタマイズ時だけ使えるようにする。
- ベースリポジトリに下流固有情報を**一切含めない**。
- テンプレートの更新漏れを防ぎつつ、ESB 側はツールを意識しない運用に近づける。

## 前提・制約
- ベースは `brand: esb` を維持（`config/branding.yaml` は esb）。
- 下流固有の値・秘密情報はベースに含めない。
- 下流リポジトリにはツールを残さない（生成物のみ固定）。
- ベース変更はパッチ取り込み前提（履歴共有が理想）。

## 決定事項（合意済み）
1) テンプレートは**外部ツールリポジトリ**にのみ置く。  
2) 下流でのブランド変更はツール側で実施し、生成物を下流で固定する。  
3) ツールの CI が **ESB のコミットをスナップショットとして保持**する。  
   - ツール側で ESB を submodule 参照し、チェックが通ったコミットに更新する。
4) ベースはツールに依存しない（コード・設定から参照しない）。

## 検討中の連携案（たたき台）
### A. ESB -> ツール CI 連携（repository_dispatch）
```
ESB CI
  └── repository_dispatch (commit SHA)
      └── ツール CI: ESB commit を submodule で checkout
          └── branding check 実行
              ├── OK: ツール repo が submodule を更新して push
              └── NG: ESB 側を fail させる（trigger-workflow-and-wait で結果取得）
```

### B. ツール側のポーリング/定期実行
ESB からの通知を無くす場合、ツール側で一定周期で ESB をチェックする方式。  
ESB 側でツールを意識しない度合いは高いが、遅延と運用コストが増える。

## 期待する成果物（将来像）
- **tool repo**:
  - generator + templates + CI
  - ESB commit を submodule で固定（CI が更新）
- **base repo**:
  - ツールを含めない（`tools/branding` などは将来的に削除）
  - `brand: esb` のみ保持（再現性の担保）
- **downstream repo**:
  - ツールは残さない
  - 生成物のみコミット

## オープン課題（次セッションで決める）
- ESB がツール連携を全く意識しない運用をどこまで追求するか  
  - repository_dispatch を使う場合、ESB CI に最小限の連携設定が残る
- ツール repo の versioning 方針（タグ / リリース / ESB commit 追従の運用）
- 「branding.lock」の要否  
  - brand・ツールバージョン・ESB commit の追跡用メタ情報として有効
- 生成物の差分検知（tool 側で `--check` 運用をどう設計するか）

## 参考: 直近のリポジトリ状況（作業の前提）
- ブランディング対応の基盤は本体に反映済み
  - E2E ランナーがブランド prefix を動的に解釈（ESB 互換エイリアスも維持）
  - CA build args の汎用化（ROOT_CA_*）
  - compose テンプレートで `*_IMAGE_TAG` のブランド参照を追加
- 直近のテストは完走済み
  - `uv run e2e/run_tests.py --reset --parallel`
  - `uv run e2e/run_tests.py --unit-only`

## 次のアクション案（新セッション）
1) ツール repo のスケルトン設計（構成/配布方法/CI 方針）
2) ESB -> ツールの連携方式を最終決定
3) tool repo が保持する ESB submodule 更新フローを定義
4) ESB 本体から `tools/branding` を段階的に削除する計画を作成
