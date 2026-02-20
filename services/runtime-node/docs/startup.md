<!--
Where: services/runtime-node/docs/startup.md
What: Startup flow for runtime-node entrypoints.
Why: Explain how runtime-node boots containerd / firecracker mode.
-->
# 起動フロー（runtime-node）

runtime-node は `entrypoint.sh` で `CONTAINERD_RUNTIME` を判定し、
containerd or firecracker の初期化を分岐します。

## 起動フロー（概略）

```mermaid
flowchart TD
    A[entrypoint.sh] --> B{CONTAINERD_RUNTIME}
    B -->|empty or containerd| C[entrypoint.containerd.sh]
    B -->|aws.firecracker| D[entrypoint.firecracker.sh]

    C --> C1[setup_cgroupv2_delegation]
    C1 --> C2[ip_forward + route_localnet]
    C2 --> C3[ensure_wg_route + watcher]
    C3 --> C4[apply_cni_nat + watcher]
    C4 --> C5[ensure_devmapper_ready]
    C5 --> C6[start_containerd]

    D --> D1[setup_cgroupv2_delegation]
    D1 --> D2[ip_forward + route_localnet]
    D2 --> D3[ensure_wg_route + watcher]
    D3 --> D4[ensure_vhost_vsock]
    D4 --> D5[start_udevd + fifo reader]
    D5 --> D6[apply_cni_nat + watcher]
    D6 --> D7[ensure_devmapper_ready]
    D7 --> D8[start_containerd]
```

## 重要ポイント
- `IMAGE_RUNTIME` は `containerd` 固定（`entrypoint.sh` でチェック）
- devmapper pool は **事前に存在**している必要があります（作成はしない）
- `apply_cni_nat` は `CNI_SUBNET` / `CNI_BRIDGE`（または `/var/lib/cni/esb-cni.env`）を参照して SNAT/FORWARD を設定します
- containerd compose では Gateway/Agent/CoreDNS が runtime-node の NetNS を共有します

---

## Implementation references
- `services/runtime-node/entrypoint.sh`
- `services/runtime-node/entrypoint.containerd.sh`
- `services/runtime-node/entrypoint.firecracker.sh`
- `services/runtime-node/entrypoint.common.sh`
