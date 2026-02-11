<!--
Where: services/runtime-node/docs/networking.md
What: Networking model for runtime-node in containerd mode.
Why: Clarify CNI/CoreDNS/NAT wiring and operational checks.
-->
# ネットワーク設計（containerd モード）

## 概要
runtime-node は CNI bridge と CoreDNS を使って worker ネットワークを提供します。

- worker subnet: `10.88.0.0/16`（CNI）
- DNS: `10.88.0.1:53`（CoreDNS sidecar）
- 外向き通信: runtime-node の iptables MASQUERADE

## 構成
```mermaid
flowchart TD
    subgraph RuntimeNS["runtime-node NetNS"]
        RN["runtime-node\ncontainerd + CNI"]
        AG["agent"]
        DNS["coredns\n10.88.0.1:53"]
        GW["gateway"]
    end

    subgraph WorkerNet["CNI bridge 10.88.0.0/16"]
        WK["worker (RIE)\n10.88.x.x"]
    end

    subgraph External["external_network"]
        S3["s3-storage"]
        DB["database"]
        VL["victorialogs"]
        REG["registry"]
    end

    AG --> RN
    RN --> WK
    WK -->|DNS| DNS
    DNS -->|forward| DockerDNS["127.0.0.11"]
    DockerDNS --> External
    WK -->|HTTP| External
    GW -->|Invoke| WK
```

## 実装上の要点
- CoreDNS は `network_mode: service:runtime-node` で runtime-node の NetNS を共有
- Agent/Gateway も containerd compose では runtime-node NetNS を共有
- NAT ルールは `apply_cni_nat()` で投入され、現状サブネットは `10.88.0.0/16` 固定

## 代表的な確認コマンド
```bash
# NAT ルール
iptables -t nat -S POSTROUTING | grep MASQUERADE

# CoreDNS ログ
docker logs <project>-coredns

# runtime-node 内 containerd 生存確認
docker exec <project>-runtime-node ctr -a /run/containerd/containerd.sock version
```

## 注意
`CNI_SUBNET` を変更しても、runtime-node 側 NAT ルールは自動追従しません。
サブネット変更を行う場合は `entrypoint.common.sh` の NAT 設定も合わせて更新が必要です。

---

## Implementation references
- `docker-compose.containerd.yml`
- `services/runtime-node/entrypoint.common.sh`
- `config/Corefile`
- `services/agent/internal/cni/generator.go`
