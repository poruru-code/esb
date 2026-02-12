<!--
Where: services/gateway/docs/network-optimization.md
What: Gateway worker-address resolution strategy.
Why: Clarify why Gateway uses worker IPs instead of DNS names for invoke path.
-->
# ネットワーク最適化（Worker 解決）

## 方針
Gateway は Agent から返される `WorkerInfo.ip_address` を invoke 経路の正本として扱います。

- Readiness check: `ip:port` へ TCP 接続
- Invoke: `ip:port/2015-03-31/functions/function/invocations` へ HTTP POST

## 理由
1. worker 起動直後に DNS 伝播待ちを避けられる
2. readiness と invoke の経路を一致させられる
3. runtime 差分（docker/containerd）を Agent 側に閉じ込められる

## 例外
- `AGENT_INVOKE_PROXY=true` の場合は Gateway -> Agent の L7 代理を使用し、
  Gateway から worker へ直接 HTTP しません。

## 注意点
- worker IP は短命です。キャッシュ前提ではなく `acquire` ごとに最新 `WorkerInfo` を使います。
- 失敗時は `LambdaInvoker` の retry/evict により worker 再取得にフォールバックします。

---

## Implementation references
- `services/gateway/services/grpc_provision.py`
- `services/gateway/services/lambda_invoker.py`
- `services/common/models/internal.py`
