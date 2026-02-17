<!--
Where: services/agent/docs/proto-generation.md
What: gRPC proto generation workflow and generated artifact locations.
Why: Keep proto contract and generated artifact paths consistent.
-->
# Proto Generation

## Source of Truth
- Proto schema: `services/contracts/proto/agent.proto`

Agent の API 契約は proto を正本とし、Go/Python の generated code は派生物です。

## Generated Outputs (Current)
| Language | Output directory | Files |
| --- | --- | --- |
| Go (Agent) | `services/agent/pkg/api/v1/` | `agent.pb.go`, `agent_grpc.pb.go` |
| Python (Gateway) | `services/gateway/pb/` | `agent_pb2.py`, `agent_pb2_grpc.py` |

## 出力配置の方針
Go 生成物は canonical path のみに配置します。

- Canonical path: `services/agent/pkg/api/v1/`
- Legacy path: `services/agent/pkg/api/v1/proto/`（再作成しない）

新規生成物を追加する場合は canonical path のみを使用してください。

## Generation Command
```bash
uv run python tools/gen_proto.py
```

処理内容:
1. Python gRPC code を `services/gateway/pb/` に生成
2. import 形式を Python package 用に補正
3. Go gRPC code を `services/agent/pkg/api/v1/` に生成（Docker 経由）

## Verification Checklist
- `services/contracts/proto/agent.proto` 変更後に `python tools/gen_proto.py` を実行
- `services/agent/pkg/api/v1/agent*.go` が更新されること
- `services/gateway/pb/agent_pb2*.py` が更新されること
- 旧パス `services/agent/pkg/api/v1/proto` に差分が発生しないこと

## Operational Notes
- `tools/gen_proto.py` は Go 生成で Docker イメージ（`rvolosatovs/protoc:latest`）を使用します。
- 生成は deterministic ではない差分（toolchain 由来）を含み得るため、
  PR では `services/contracts/proto/agent.proto` と generated files をセットでレビューします。

---

## Implementation references
- `services/contracts/proto/agent.proto`
- `tools/gen_proto.py`
- `services/agent/pkg/api/v1/agent.pb.go`
- `services/agent/pkg/api/v1/agent_grpc.pb.go`
- `services/gateway/pb/agent_pb2.py`
- `services/gateway/pb/agent_pb2_grpc.py`
