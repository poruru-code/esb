# WireGuard config (gateway/compute)

This directory ships **example** configs. You can still copy them manually, but
`esb node provision` now **generates keys and configs automatically** if
`wireguard-tools` is installed on the control host.
If you pass `--wg-conf`, auto-generation is skipped and that file is used as-is.

If the command prints "WireGuard tools not found", install:
```bash
sudo apt-get update && sudo apt-get install -y wireguard-tools
```

## Generate keys (manual)

```bash
wg genkey | tee gateway.key | wg pubkey > gateway.pub
wg genkey | tee compute.key | wg pubkey > compute.pub
```

## Place configs (manual)

```bash
mkdir -p <repo_root>/.<brand>/wireguard/gateway <repo_root>/.<brand>/wireguard/compute
cp config/wireguard/gateway/wg0.conf.example <repo_root>/.<brand>/wireguard/gateway/wg0.conf
cp config/wireguard/compute/wg0.conf.example <repo_root>/.<brand>/wireguard/compute/wg0.conf
```

Edit both files:

- Replace `<GATEWAY_PRIVATE_KEY>` / `<GATEWAY_PUBLIC_KEY>`
- Replace `<COMPUTE_PRIVATE_KEY>` / `<COMPUTE_PUBLIC_KEY>`
- Replace `<COMPUTE_VM_IP>` and `10.88.x.x` values for your node

## Auto-generate with `esb node provision`

```bash
esb node provision \
  --host esb@10.1.1.220 \
  --wg-subnet 10.88.1.0/24 \
  --wg-runtime-ip 172.20.0.10
```

Output files:
- Gateway: `<repo_root>/.<brand>/wireguard/gateway/wg0.conf`
- Compute: `<repo_root>/.<brand>/wireguard/compute/<node-name>/wg0.conf`

Note:
- `--wg-runtime-ip` は **Compute VM の docker bridge 内**にいる `runtime-node` 固定IPです。
  `docker-compose.node.yml` の `runtime_net` を `172.20.0.0/16` / `172.20.0.10` で運用する想定です。
  `wg0.conf` の `PostUp/PostDown` で `ip route` を best-effort にしておくと、runtime-node 起動前でも WG の起動が継続できます。
- WG 経由で Lambda から Gateway へ戻す場合は以下を設定します:
  - `GATEWAY_INTERNAL_URL=https://10.99.0.1:443`
  - `WG_CONTROL_NET=10.99.0.0/24`（runtime-node 内にルート追加）
- MTU/PMTUD 対策として、生成される `wg0.conf` に MTU と MSS clamping を含めています。

## MTU tuning

If TLS stalls over WireGuard (WSL/Hyper-V などで起きやすい), lower the WG MTU:

```bash
WG_MTU=1340 esb node provision --host esb@10.1.1.220
```

You can also edit the configs directly and restart WireGuard on both sides.
