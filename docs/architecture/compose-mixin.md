# Docker Compose アーキテクチャ (Mixin Strategy)

ESB (Edge Serverless Box) では、ランタイムモード（Containerd / Firecracker）やデプロイ構成の違いを柔軟に吸収するため、**Mixin（積み上げ）方式** の Docker Compose 構成を採用しています。

## 概要

単一の巨大な `docker-compose.yml` を条件分岐させるのではなく、**役割ごとの小さなYAMLファイル** を定義し、モードに応じて CLI が適切に組み合わせる（Mixin）ことで最終的な構成を決定します。

これにより、以下のメリットが得られます：
1.  **Firecracker と Containerd の完全分離**: 不要な `runtime-node` コンテナが Containerd モードで起動しません。
2.  **YAML の簡素化**: 各ファイルは単一の役割に集中できます。
3.  **柔軟性**: 将来的に別のランタイムを追加する場合も、新しい Adapter ファイルを作るだけで済みます。

## ファイル構成と役割

| ファイル名 | 役割 | 必須 | 説明 |
|------------|------|------|------|
| **`docker-compose.yml`** | **Core** | ✅ | コントロールプレーン（Gateway, DB, Registry, S3, etc）を提供します。ランタイムに依存しません。 |
| **`docker-compose.worker.yml`** | **Worker** | ✅ | **Agent の基本定義**（Image, Env, Volumes）を提供します。**ネットワークやポート設定を含みません**。純粋なコンピュートユニットの定義です。 |
| **`docker-compose.fc.yml`** | **FC Adapter** | (択一) | **Firecracker モード用**のアダプターです。<br>1. `runtime-node` (Firecracker VM Manager) を定義します。<br>2. Agent を `runtime-node` の Sidecar (Network Namespace共有) として上書きします。<br>3. Gateway の接続先を `runtime-node:50051` に設定します。 |
| **`docker-compose.containerd.yml`** | **Ctr Adapter** | (択一) | **Containerd モード用**のアダプターです。<br>1. Agent に独自のネットワーク設定とポート (`50051`) を付与し、完全な Standalone として動作させます。<br>2. Gateway の接続先を `agent:50051` に設定します。 |

## モード別の構成図

### Pattern A: Firecracker Mode
**組み合わせ**: `[ Core + Worker + FC Adapter ]`

`runtime-node` がホストとなり、Agent はその中に「寄生」する形で動作します。Gateway は親である `runtime-node` と通信します。

```mermaid
graph TD
    subgraph FC_Adapter ["docker-compose.fc.yml"]
        Runtime[runtime-node<br>(Host)]
        Agent[agent<br>(Sidecar / network: service)]
        Agent -.-> Runtime
    end
    
    Gateway[Gateway]
    Gateway -- "gRPC (runtime-node:50051)" --> Runtime
```

### Pattern B: Containerd Mode (Default)
**組み合わせ**: `[ Core + Worker + Ctr Adapter ]`

Agent は独立したコンテナとして動作し、ネットワークに直接参加します。

```mermaid
graph TD
    subgraph Ctr_Adapter ["docker-compose.containerd.yml"]
        Agent[agent<br>(Standalone)]
    end
    
    Gateway[Gateway]
    Gateway -- "gRPC (agent:50051)" --> Agent
```

## 運用方法

CLI (`esb up`) が自動的にモードを検出し、適切なファイルを `-f` オプションで渡します。ユーザーが手動でファイルを指定する必要はありません。

### 手動実行の場合（デバッグ用）

**Containerd Mode:**
```bash
docker compose -f docker-compose.yml -f docker-compose.worker.yml -f docker-compose.containerd.yml up -d
```

**Firecracker Mode:**
```bash
docker compose -f docker-compose.yml -f docker-compose.worker.yml -f docker-compose.fc.yml up -d
```
