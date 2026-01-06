<!--
Where: docs/network-routing.md
What: Network routing overview and DNAT/WG behavior for ESB.
Why: Keep routing expectations and troubleshooting steps in one place.
-->
# ネットワーク/ルーティング概要

このドキュメントは、Edge Serverless Box の「local-proxy + DNAT」構成における
ネットワーク設計を説明します。現行の compose では gateway は Control 側、
runtime-node/agent/local-proxy は Compute 側に分離され、worker から gateway への戻りは
WireGuard 経由を前提とします。

## スコープと目的

- Lambda SDK/内部呼び出しのための `10.88.0.1` 互換を維持する
- 外部サービス（S3/DB/Logs）の固定 IP 依存を撤廃する
- DNAT を local L4 proxy（HAProxy）に集約し、Docker DNS でバックエンド解決する
- Phase C（Firecracker）への互換性を維持する

## コンポーネントと役割

- runtime-node:
  - containerd + CNI bridge（`10.88.0.0/16`）を実行
  - DNAT 用 iptables ルールを管理
  - agent / local-proxy と NetNS を共有
- local-proxy（HAProxy）:
  - runtime-node の NetNS 内で `127.0.0.1:9000/8001/9428` にバインド
  - Docker DNS で `s3-storage` / `database` / `victorialogs` を解決して TCP 転送
- gateway:
  - HTTPS エントリポイント（`:443`）
  - `worker.ip:8080` へ Invoke
  - `VICTORIALOGS_URL` へログ送信（既定は `http://victorialogs:9428`）
- agent:
  - containerd 経由で task を作成し CNI 接続
- worker（Lambda コンテナ）:
  - boto3 のエンドポイント:
    - S3: `http://10.88.0.1:9000`
    - DynamoDB: `http://10.88.0.1:8001`
    - Logs: `http://10.88.0.1:9428`

## Network Namespace

- runtime-node の NetNS を共有するもの:
  - agent
  - local-proxy
- gateway は Control Plane 側の独立コンテナで起動する
- Lambda worker は CNI bridge（`10.88.0.0/16`）へ接続
- worker のIP割り当て範囲は `CNI_SUBNET` で node ごとに固定する（例: `10.88.1.0/24`）
  - CNI bridge の base subnet は `10.88.0.0/16` のまま維持し、`10.88.0.1` 互換は継続する
- 単一ノード構成では `docker-compose.containerd.yml` が runtime-node を external_network に参加させる

## Gateway 側 WireGuard ルート補正（C-1.6）

- gateway 起動時に `wg-quick` でトンネルを上げた後、`wg showconf wg0` の `AllowedIPs` を読み取り
  `ip route replace <CIDR> dev wg0` を実行してルートを補正する
- 複数ノードで `AllowedIPs` が増えても、期待経路（`10.88.x.0/24` など）が `wg0` に確実に乗るようにする
- 危険な CIDR はフィルタする（`0.0.0.0/0`、`169.254.0.0/16`、`224.0.0.0/4`、IPv6 など）
  - 許可対象は RFC1918 サブネットと `/32` のホストルート
- ローカル検証で worker 経路を runtime-node 側に寄せる場合は、
  `GATEWAY_WORKER_ROUTE_VIA_HOST=runtime-node` を設定して
  `10.88.0.0/16` のみ次ホップを上書きする

## トラフィックフロー

1) Client -> Gateway（HTTPS）
- 経路: Host `:443` -> gateway

2) Gateway -> Worker（Invoke）
- 経路: gateway -> `worker.ip:8080`

3) Worker -> Gateway（Lambda chain invoke）
- 既定（WG）: worker -> `https://10.99.0.1:443`
- 旧構成（gateway が runtime-node NetNS 上）: worker -> `https://10.88.0.1:443`

4) Worker -> S3 / DynamoDB / VictoriaLogs
- 経路: worker -> `10.88.0.1:9000|8001|9428`
- iptables DNAT -> `127.0.0.1:9000|8001|9428`
- local-proxy -> DNS でバックエンド転送（`s3-storage`, `database`, `victorialogs`）

5) Gateway -> VictoriaLogs
- 経路: gateway -> `VICTORIALOGS_URL`（既定は `http://victorialogs:9428`）

## DNAT ルール（runtime-node）

runtime-node の entrypoint が以下を設定します:

- `10.88.0.1:9000` -> `127.0.0.1:9000` -> local-proxy -> `s3-storage:9000`
- `10.88.0.1:8001` -> `127.0.0.1:8001` -> local-proxy -> `database:8000`
- `10.88.0.1:9428` -> `127.0.0.1:9428` -> local-proxy -> `victorialogs:9428`

適用範囲:
- PREROUTING（worker からの CNI トラフィック）
- OUTPUT（runtime-node NetNS 内のトラフィック）

注意: `127.0.0.1` への DNAT には `route_localnet=1` が必須です。  
`services/runtime-node/entrypoint.sh` で設定しています。

## 主要な環境変数

- `CNI_GW_IP`（既定: `10.88.0.1`）
- `DNAT_S3_IP`, `DNAT_DB_IP`, `DNAT_VL_IP`
  - local-proxy モード: `127.0.0.1`
  - 空文字の場合は DNAT ルールを作成しない
- `DNAT_DB_PORT`（既定: `8000`）
- `DNAT_DB_DPORT`（既定: `8001`）
- `DNAT_APPLY_OUTPUT`（既定: `1`）
- `GATEWAY_INTERNAL_URL`（既定: `https://10.99.0.1:443`。旧構成は `https://10.88.0.1:443`）
- `VICTORIALOGS_URL`（既定: `http://victorialogs:9428`）

## local-proxy（HAProxy）設定

設定ファイル: `config/haproxy.cfg`

- バインド:
  - `127.0.0.1:9000` -> `s3-storage:9000`
  - `127.0.0.1:8001` -> `database:8000`
  - `127.0.0.1:9428` -> `victorialogs:9428`
- Docker DNS（`127.0.0.11`）を利用

## Registry (Firecracker/Compute -> Control)

> [!IMPORTANT]
> 本セクションは、レジストリを介してイメージを配布する Firecracker モードおよび Containerd モード専用の仕様です。Docker モードではレジストリを使用せず、サポートもされません。

- Compute 側の image pull は **WG 経由で `10.99.0.1:5010`** に到達する前提。
- Gateway コンテナ内で HAProxy を起動し、`10.99.0.1:5010 -> esb-registry:5010` を中継する。
  - 設定ファイル: `config/haproxy.gateway.cfg`
- レジストリの TLS 証明書には `10.99.0.1` の SAN が必要。
- Compute 側は以下の CA 信頼が必須:
  - Docker: `/etc/docker/certs.d/10.99.0.1:5010/ca.crt`
  - runtime-node: `/usr/local/share/ca-certificates/esb-rootCA.crt` + `update-ca-certificates`

## Control/Compute の名前解決

- `docker-compose.node.yml` は `extra_hosts` で `esb-registry` / `s3-storage` / `database` / `victorialogs` / `gateway`
  を `ESB_CONTROL_HOST` に解決する。
- `ESB_CONTROL_HOST` は Compute から到達可能な Control のIP/ホストを指定する（WG の場合は `10.99.0.1`）。

## トラブルシュート

1) DNAT ルール確認
```
docker exec esb-runtime-node iptables -t nat -S
docker exec esb-runtime-node iptables -t nat -L PREROUTING -n -v
```

2) route_localnet（1であること）
```
docker exec esb-runtime-node cat /proc/sys/net/ipv4/conf/all/route_localnet
```

3) local-proxy の解決/疎通確認
```
docker logs --tail=200 esb-local-proxy
```

4) runtime-node NetNS から DNAT を確認
```
docker exec esb-runtime-node curl -f http://10.88.0.1:9000/health
```

5) worker から gateway 到達確認（WG 構成時）
```
docker exec esb-runtime-node curl -k https://10.99.0.1/health
```

## Phase C への影響

- DNAT + local-proxy モデルは Phase C でも維持する
- worker がコンテナから microVM に変わるだけ
- `10.88.0.1` 互換は必須条件のまま
