**ChaosBlade 服务化设计方案（PoC 优先）**

版本：v0.1

目标：将 `chaosblade` 改造为长期运行的服务（HTTP API），使服务能够在进程内完成故障注入与销毁（默认使用进程内执行），初期使用固定 token 做认证，数据库使用 SQLite，使用 `gin` 作为 HTTP 框架，使用 `gorm` 访问 SQLite。

**一、总体设计要点**
- **Server（Control Plane）**：长期运行的 HTTP 服务，接收 REST 请求并调度注入任务。
- **Executor 层（在进程内执行）**：重构 `exec/*` 包以支持“进程内执行”接口，避免依赖外部二进制。对于难以移除二进制的场景，保留外部二进制作为回退模式。
- **Job 管理器**：负责异步执行、超时控制、状态持久化、回调、重试与清理。
- **存储**：使用 `SQLite`（通过 `gorm`），存储实验元信息、状态和审计日志。
- **认证**：PoC 使用静态固定 token（HTTP Header，如 `X-Api-Token`），并保留扩展到 JWT/API Key/RBAC 的接口。

**二、技术栈**
- HTTP 框架：`gin-gonic/gin`
- DB ORM：`gorm.io/gorm` + `gorm.io/driver/sqlite`
- 任务队列：内存队列 + SQLite 持久化（PoC）。后期可替换为 Redis/Message Queue。
- 日志：继续使用项目现有的 `spec-go/log`，并在 server 层接入结构化日志。

**三、REST API 设计（示例）**
- POST `/v1/experiments` — Create
  - 请求 JSON：
    - `target` (string), `action` (string), `scope` (string), `flags` (object), `async` (bool), `timeout` (int 秒), `callback` (string 可选)
  - 返回：{ code, message, data: { uid } }
- DELETE `/v1/experiments/{uid}` — Destroy
- GET `/v1/experiments/{uid}` — Query 状态
- GET `/v1/experiments` — 列表（可按 status/target 过滤）

认证（PoC）：
- 客户端需在请求头中携带 `X-Api-Token: <fixed-token>`。
- 服务器配置文件/环境变量 `BLADE_API_TOKEN`。

示例请求：
```
curl -X POST -H "Content-Type: application/json" -H "X-Api-Token: secret-token" \
  -d '{"target":"cpu","action":"load","scope":"host","flags":{"cpu-percent":"60"},"async":false,"timeout":120}' \
  http://127.0.0.1:9526/v1/experiments
```

**四、DB Schema（SQLite / GORM 模型草案）**
- Experiment 表（experiments）
  - id (auto)
  - uid (string, unique)
  - target (string)
  - action (string)
  - scope (string)
  - flags (text/json)
  - status (string) // created/running/success/failed/destroyed
  - err_msg (text)
  - created_at, updated_at, started_at, finished_at
  - timeout (int)
  - callback (string)
  - executor_mode (string) // inprocess|external

建议在 `data/` 下新增或扩展现有持久化层，使用 GORM 的 auto-migrate 完成初始表结构。

**五、Executor 改造策略**
1. 在 `spec` 或 `exec` 包增加接口扩展（兼容现有代码）：
   - 保持 `Exec(uid, ctx, model)` 不变
   - 新增 `ExecInProcess(uid, ctx, model) (*spec.Response)`，默认实现可以调用原 `Exec`（回退保留）
2. 将 `target` 各自实现（例如 `exec/os`）的核心逻辑提取到可复用库（如 `exec/os/core` 或在 `exec/os` 内暴露 `RunCore` 函数），使得：
   - `target` 独立二进制的 `main.go` 调用 RunCore
   - Server 的 `ExecInProcess` 调用 RunCore（无需 fork/exec）
3. 若某些操作必须在独立进程运行（例如与特权 C/C++ 模块强耦合），保留 `EXEC_MODE=external`，Server 可以通过配置回退到调用外部二进制（当前已有实现可复用）。

**六、Job 管理器与超时策略**
- Job Worker：简易 goroutine 池，取任务并执行 `ExecInProcess`。
- 超时：根据 `timeout` 字段启动定时器，超时后触发 destroy（或设置状态并调用回调）。
- 异步：若 `async=true`，返回 UID 后异步执行；若 `async=false`，同步返回执行结果。

**七、审计与日志**
- 每次 API 调用记录审计信息（请求者 IP、token、参数、时间）。
- executor 运行日志按实验分文件或写入集中日志，便于排查。

**八、安全考虑**
- 初期使用静态 token，生产需扩展为 API Key/JWT + RBAC 模型。
- Server 运行用户应最小化权限，尽量避免以 root 直接运行；必要特权可通过部署策略（Pod Security、Capability 授权）控制。

**九、配置与环境**
- 主要配置项（支持环境变量与配置文件）：
  - `BLADE_API_TOKEN`（必需，PoC）
  - `BLADE_PORT`（默认 9526）
  - `BLADE_EXEC_MODE`（inprocess|external，默认 inprocess for PoC 可配置）
  - `BLADE_SQLITE_PATH`（默认 `./chaosblade.db`）

**十、PoC 实施计划（优先级）**
步骤 A — 最小 HTTP 接口 + 固定 token + SQLite：
  1. 在仓库新增 `server/` 包或修改 `cli/cmd/server_start.go` 的 `start0()`，用 `gin` 注册路由 `/v1/experiments`。
  2. 建立 GORM + SQLite 连接（`data/sqlite.go`），并创建 `experiments` 表（GORM auto-migrate）。
  3. POST `/v1/experiments` 解析请求后：
     - 构造 `spec.ExpModel`，创建 DB 记录（status=created，生成 UID）。
     - 直接调用 `exec/os` 的 `ExecInProcess`（如无则在 `exec/os` 中快速适配：新增 `ExecInProcess`，内部调用 `exec/os/core` 函数）
     - 根据执行结果更新 DB（status success/failed），返回 JSON。
  4. DELETE `/v1/experiments/{uid}` 调用 `ExecInProcess` 的 destroy 模式（或调用 `Exec` 并设置 Destroy 上下文），更新 DB。

步骤 B — 增加异步、超时、回调和固定 token 校验。

步骤 C — 编写单元测试与集成测试，验证在受控环境下的真实注入。

**十一、向后兼容与回退**
- PoC 仅改变 server 行为，CLI 本地行为不变。保留通过 `os/exec` 调用 `chaosblade-exec-*` 的路径作为回退。
- 增量迁移：先以 `EXEC_MODE=external` 作为默认（不影响现有部署），逐步将 `inprocess` 作为可选并测试后切换默认。

**十二、部署建议**
- 单机：以二进制/容器运行 `blade server start`，并以 `systemd` 或容器管理进程。
- 大规模：在每个目标主机上以 DaemonSet/agent 模式部署（以保证本地注入权限和最小网络边界）。

**十三、测试计划（概述）**
- 单元测试：Executor 内核函数、DB 存取、API 校验。
- 集成测试：Server 接口到 Executor 的 E2E（沙箱 VM/container）。
- 安全测试：Token 泄露与权限检查、命令注入、资源隔离测试。

**十四、风险与限制**
- 权限风险：进程内执行需要更高权限，增加攻击面；需要通过部署策略与审计来控制。
- 兼容风险：已有使用外部二进制实现的功能可能需要较多重构。

**十五、后续扩展**
- 认证升级：支持 JWT、API Key、RBAC。保留中间件接口以便替换。
- 多租户/配额：支持租户 ID 与资源配额。
- 分布式调度：server 作为 control plane，配合节点 agent 实现集中管理。

——

下一步（如果你同意）：我会实现 PoC 的最小变更：
- 在 `cli/cmd/` 或新 `server/` 包里用 `gin` 加入 `/v1/experiments` POST/DELETE
- 使用 `gorm` + SQLite 保存 experiment 元数据
- 对 `exec/os` 做最小适配，增加 `ExecInProcess` 路径并调用已有核心逻辑

你是否同意我现在开始实现 PoC？
