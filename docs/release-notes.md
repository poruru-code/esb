# Release Notes

## 2026-02-24

### Breaking Change: runtime-config path env 廃止
- 旧 runtime-config パス環境変数は公開仕様・実行仕様から削除されました。
- `docker compose up -d` 後の runtime-config は常に named volume `esb-runtime-config` を使用します。
- `artifactctl deploy` / `esb deploy` は staging から実行中 compose stack の runtime-config mount を解決して同期します。
- 旧運用（runtime-config パス指定、`artifactctl deploy --out`）はサポート対象外です。
