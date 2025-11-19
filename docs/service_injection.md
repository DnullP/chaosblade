# Service-Based Fault Injection

This project now exposes fault injection through HTTP and gRPC services instead of relying on the original standalone CLI binary.
The service process runs as a daemon (for example in a DaemonSet) and performs injection directly inside target nodes or containers.

## High-Level Flow
1. The server boots from `cmd/server/main.go`, builds a dispatcher, loads default executors for `os`, `jvm`, and `cri`, and starts HTTP/gRPC listeners.
2. The HTTP router in `pkg/server/http/router.go` exposes `/api/v1/experiments` for create/destroy, `/api/v1/preparations` for JVM agent prepare/revoke, and `/api/v1/status` for querying records.
3. Experiment creation calls `pkg/service/experiment/service.go` which persists a record to SQLite through the GORM-backed data source, then dispatches the request to the appropriate executor.
4. The dispatcher (`pkg/service/dispatcher`) maps the scope/target/action triplet to executors registered from spec YAML definitions via `LoadDefaultExecutors`.
5. Executors from `exec/os`, `exec/jvm`, and `exec/cri` implement the real fault injection logic. For `cri`, the executor uses nsexec to enter the target container namespaces before applying the underlying OS or JVM fault actions, matching the original CLI behavior.
6. Destroy requests set a destroy flag in the context so executors can roll back stateful faults.

## HTTP Usage Overview
- **Create experiment**: `POST /api/v1/experiments` with `target`, `action`, `scope` (optional), and `flags` to trigger injection. Returns the experiment UID and record.
- **Destroy experiment**: `DELETE /api/v1/experiments/{uid}` (optionally include `target`, `action`, and `flags` if the record is missing) to roll back.
- **Query status**: `GET /api/v1/status?type=create&uid=<uid>` or filter by target/action/flag/status for lists.

The service performs all actions within the long-running process; no external binary calls are required once the executors are registered and channels are set to the local host.
