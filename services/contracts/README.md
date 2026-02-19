<!--
Where: services/contracts/README.md
What: Ownership and scope for cross-service API contracts.
Why: Keep service boundary contracts explicit and avoid repo-root coupling.
-->
# Services Contracts

`services/contracts` contains API contracts shared by service components.

Current scope:
- `services/contracts/proto/agent.proto`: gRPC contract between Gateway and Agent.

Rules:
1. Contracts here must be consumed by at least two service modules.
2. Generated artifacts live with each service (`services/agent/pkg/api/v1`, `services/gateway/pb`).
3. Root-level `contracts/` is retired for this repository layout.

Generation entrypoint:
- `mise run gen-proto`
- `buf generate services/contracts`
