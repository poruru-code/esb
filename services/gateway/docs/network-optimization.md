<!--
Where: services/gateway/docs/network-optimization.md
What: Network resolution strategy for Gateway invokes (IP vs DNS).
Why: Record the rationale and code paths for the hybrid approach.
-->
# 技術決定事項: ネットワーク解決のハイブリッド最適化

## 背景
Docker コンテナ間の通信において、本プロジェクトは「Lambda の冷間起動（Cold Start）」の高速化と、システム全体の「保守性・堅牢性」の両立を目指しています。

現在は **IP アドレスを優先する設計**を採用しています。Agent は runtime から取得した IP を返却し、Gateway はその IP で Readiness チェックと Lambda Invoke を行います。

## 解決策: ハイブリッドアプローチ

### 1. 起動確認（Readiness Check）: **IP アドレスを使用**
Agent から受け取った IP アドレスに対して TCP 接続を行い、コンテナの起動完了を検知します。
- **理由**: Docker DNS の伝播（数ミリ秒〜数秒）を待たずに、コンテナがネットワーク的に疎通可能になった瞬間を最速で検知するため。
- **効果**: 開発者が体感する Cold Start の待ち時間を理論上の最速値まで短縮します。

### 2. Gateway からの通信: **IP アドレスを使用**
Gateway は `WorkerInfo.ip_address` を直接使用して Lambda RIE に接続します。
- **理由**: CNI/Docker のネットワーク上の IP を即座に利用でき、DNS 伝播待ちを不要にするため。
- **メリット**: Readiness チェックと実行パスを統一でき、起動直後の呼び出し遅延を最小化できます。

## 根拠と検証データ

### 性能比較
| 解決方法 | 平均レイテンシ | 役割 |
| :--- | :--- | :--- |
| **IPアドレス (直接指定)** | **0.092 ms** | **Readiness / Invoke で使用** (最速の検知) |
| **ホスト名 (DNS経由)** | **0.595 ms** | **サービス解決で使用** (ワーカーの外部通信) |

この差（約 0.5ms）は、アプリケーション全体の処理時間（数百 ms 〜）と比較すると無視できる範囲であり、DNS を利用することによる保守性向上のメリットが上回ると判断しました。

## 実装の詳細
`services/gateway/services/grpc_provision.py` および `services/gateway/services/lambda_invoker.py` では以下のフローを実行します：

1. Agent にコンテナ起動を依頼し、`WorkerInfo.ip_address` を取得。
2. 取得した IP アドレスに対して `_wait_for_readiness(host, port)` を実行。
3. 確認完了後、Gateway は **IP アドレス** 宛にリクエストを送信。

---

## Implementation references
- `services/gateway/services/grpc_provision.py`
- `services/gateway/services/lambda_invoker.py`
