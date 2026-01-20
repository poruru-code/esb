# 企業プロキシ環境での利用ガイド

Edge Serverless Box を企業プロキシ環境で利用する際の設定と挙動をまとめます。
できるだけ CLI 側で自動適用されますが、ホストや Docker デーモンの設定が必要な場合は
本ガイドに従ってください。

## 自動適用される内容

- CLI は `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`（大小文字を含む）を検出し、
  `NO_PROXY` に以下の内部宛先を追加して Docker Compose / docker build に伝搬します。
  - `localhost`, `127.0.0.1`, `::1`, `registry`, `gateway`, `runtime-node`, `agent`,
    `s3-storage`, `database`, `victorialogs`, `local-proxy`,
    `10.88.0.0/16`, `10.99.0.1`, `172.20.0.0/16`
- `esb up` / `esb down` / `esb stop` などの Docker Compose 実行時に `NO_PROXY` を注入し、
  ローカル宛先へのアクセスがプロキシ経由にならないようにします。
- `esb build` / 自動リビルドは docker build にプロキシを build-arg として渡します。
- Gateway 内の httpx クライアントは `trust_env=False` / `proxies=None` で生成されるため、
  環境変数由来のプロキシを見に行かず、ハートビートなどの内部通信はプロキシ経由になりません。

### 追加の `NO_PROXY`

環境固有のアドレスを除外したい場合は `ESB_NO_PROXY_EXTRA` をカンマ区切りで指定すると、
CLI/プロビジョニングの `NO_PROXY` に追記されます。

## ホスト側で必要な設定（例）

CLI の自動設定ではカバーできない、ホスト Docker デーモン向けの設定例です。

### Docker CLI / デーモンにプロキシを設定する

`~/.docker/config.json` に以下を追加すると、CLI/デーモンの双方でプロキシが利用できます。

```json
{
  "proxies": {
    "default": {
      "httpProxy": "http://proxy.corp.example:3128",
      "httpsProxy": "http://proxy.corp.example:3128",
      "noProxy": "localhost,127.0.0.1,.corp.internal"
    }
  }
}
```

プロキシ証明書が社内 CA の場合は、ホスト側の信頼ストアにインポートしてください。

### システムのプロキシ環境変数

CI やシェル全体でプロキシを強制したい場合は、`/etc/environment` などに
`HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` を追記してください。
CLI は既存の設定を尊重しつつ内部宛先を `NO_PROXY` に追加します。

## トラブルシュート

- Docker イメージの取得に失敗する場合は `docker info` の出力にプロキシ設定が載っているか確認し、
  `~/.docker/config.json` または systemd drop-in を見直してください。
- リモートノード側で apt/curl がタイムアウトする場合は、対象ノードのプロキシ設定
  （`/etc/apt/apt.conf.d` や `/etc/profile.d` など）を確認し、
  `ESB_NO_PROXY_EXTRA` に WireGuard 網や社内 DNS のアドレスを追加してください。
