# CRI 迁移与进程内 nsexec 实现细化说明

目标：在服务化改造中，CRI 类型的故障注入要在 server 进程内完成容器切换（namespace 切换）后执行 `os` 或 `jvm` 类型的注入逻辑，避免外部二进制调用。此文档给出实现要点、权限要求、代码示例和集成建议。

概述
- CRI 的本质是定位目标容器（container id / pod），找到其主进程 PID，然后在 server 进程内进入目标容器的命名空间（pid, net, mnt, ipc, uts 等），在该命名空间内执行 `os` 或 `jvm` 的注入逻辑。
- 技术要点：在 Go 中调用 `setns` 必须在固定的 OS 线程上进行（`runtime.LockOSThread()`），并考虑返回前恢复原有命名空间或退出线程。

高层实现步骤
1. 接收 API 请求并解析参数：container id/pod/ns/target/action/flags、uid、timeout 等。
2. 定位目标容器的 PID：
   - 如果输入直接是容器 PID 或容器 ID，可通过 CRI runtime（containerd/cri或docker）查询容器状态并获取 PID。
   - 简单实现（DaemonSet 模式）：在节点上可通过 `crictl` 或读取 CRI socket（containerd/docker shim）获得 PID，或者通过 `/proc` 与 cgroup 信息匹配容器进程。
3. 准备要在目标命名空间中运行的函数（注入逻辑）：
   - 把 `exec/os` 和 `exec/jvm` 的核心运行逻辑提取/包装为可调用的函数（例如 `RunOSAction(ctx, model)`、`RunJVMAction(ctx, model)`）。
4. 在一个新 goroutine 中锁定 OS 线程并执行 `setns`：
   - `runtime.LockOSThread()`
   - 打开目标 PID 的 namespace 文件（例如 `/proc/<pid>/ns/net`）并调用 `syscall.Setns(fd, 0)`
   - 调用要执行的注入逻辑（RunOSAction / RunJVMAction）
   - 退出 goroutine 或按需恢复原命名空间（通常可直接退出线程，使得线程死亡后新 goroutine 使用新线程）
5. 收集并返回注入结果；如果是异步任务，向 Job 管理器登记并立即返回 UID。

权限与部署要求
- 需要 root 或具备足够的能力（例如 CAP_SYS_ADMIN）来调用 `setns`。因此 server 需要以特权模式运行（DaemonSet 大多数场景下需要 `privileged: true` 或特定 capabilities）。
- 推荐部署方式：以 DaemonSet 在每个节点本地运行服务（server 既是 control plane 也是 node agent），或将 server 作为管理进程配合轻量 agent。

容器 PID 定位策略（可选方案）
- 优先方案：调用 CRI runtime API（containerd CRI / Docker Engine API）获取 container PID。
- 兼容方案：解析 `/proc` 列表并通过 cgroup 信息识别容器进程（适用于容器运行时不提供易用 API 的情况）。
- 最简单 PoC：要求请求方提供容器主进程 PID 或容器内可识别的进程标识（由上层系统负责解析）。

在进程内调用 setns 的 Go 代码示例
下面是一个封装函数示例，用于在目标 PID 的若干命名空间中执行一个回调函数：

```go
package ns

import (
    "fmt"
    "os"
    "path/filepath"
    "runtime"
    "syscall"
)

// runInNamespaces 在指定 pid 的命名空间中运行 f。namespaces 可以是 ["net","pid","mnt","ipc","uts"]。
func runInNamespaces(targetPid int, namespaces []string, f func() error) error {
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    // open namespace fds
    origNS := make(map[string]*os.File)
    defer func() {
        for _, fh := range origNS {
            if fh != nil {
                fh.Close()
            }
        }
    }()

    // Optionally save current ns fds if you want to restore (omitted for brevity)

    for _, ns := range namespaces {
        nsPath := filepath.Join("/proc", fmt.Sprintf("%d", targetPid), "ns", ns)
        fh, err := os.Open(nsPath)
        if err != nil {
            return fmt.Errorf("open ns %s failed: %v", nsPath, err)
        }
        // perform setns
        if err := syscall.Setns(int(fh.Fd()), 0); err != nil {
            fh.Close()
            return fmt.Errorf("setns %s failed: %v", nsPath, err)
        }
        origNS[ns] = fh
    }

    // Now we are in the target namespaces on this OS thread
    return f()
}
```

注意事项：
- `syscall.Setns` 需要相应权限；且 setns 只影响当前线程（hence LockOSThread）。
- 恢复原命名空间可以通过在开始前打开当前线程的 namespace fd（例如 `/proc/self/ns/net`），在结束时再调用 `setns` 恢复。但一种更简单的策略是把此线程作为短生命周期线程（执行完回调后退出），让 Go 的调度分配新的线程给其他 goroutine。务必避免长期在同一线程中保留容器命名空间，这会影响全局运行态。

如何将 `exec/os` / `exec/jvm` 与 CRI 集成
- 把 `exec/os` 的命令执行核心封装为可在当前进程调用的函数（例如 `RunOSAction(ctx, model)`），该函数内部执行原先的命令准备、系统调用、内核交互等。
- 对于 jvm：注入通常需要在容器內访问 Java 进程（例如使用 `jattach`、`jcmd`、或 agent），因此必须在 target 容器命名空间中执行 jvm 的核心函数，使其能看到容器內进程。
- CRI Executor 的 `ExecInProcess`（新接口）职责：
  1. 根据 model 中的 container 标识解析出 targetPid
  2. 根据需要选择要进入的命名空间集合（最少 pid/net/mnt）
  3. 在 setns 环境中调用对应的 `RunOSAction` 或 `RunJVMAction`

错误处理与超时
- 在调用 runInNamespaces 时，应该设置上下文（`context.Context`）并在回调中尊重 ctx.Done()。
- 若操作需要后台长期运行（process hang），应在 Job 管理器中记录 pid 并用随后的 destroy 调用进行清理。

并发与资源限制
- 由于 `setns` 的线程级别限制，应该控制并发 setns 的数量（可通过 semaphore 限制并发进入 namespace 的线程数），避免线程资源耗尽。

回退策略
- 如果在某些环境下无法在进程內安全地进行 setns（权限不足、API 不可用），Server 应支持回退到调用外部 `chaosblade-exec-cri` 二进制（在该二进制里完成 nsexec），或通过 Node Agent 方式触发注入。

安全与审计
- 对容器切换与注入操作做严格的审计记录：调用者、token、目标容器、时间、参数与结果。

示例：CRI Executor `ExecInProcess` 伪代码
```go
func (e *Executor) ExecInProcess(uid string, ctx context.Context, model *spec.ExpModel) *spec.Response {
    // 1. 解析 container id / pod 信息，定位 pid
    pid, err := resolveContainerPid(model)
    if err != nil {
        return spec.ResponseFail(...)
    }

    // 2. 按需进入 target namespace 并执行
    err = ns.runInNamespaces(pid, []string{"pid","net","mnt"}, func() error {
        // choose underlying executor: os or jvm
        if model.Target == "os" {
            resp := oscore.RunOSAction(uid, ctx, model)
            // convert resp to error or handle result
            return respToError(resp)
        } else if model.Target == "jvm" {
            resp := jvmcore.RunJVMAction(uid, ctx, model)
            return respToError(resp)
        }
        return nil
    })
    if err != nil {
        return spec.ResponseFail(...)
    }
    return spec.ReturnSuccess(...)
}
```

测试建议（PoC 阶段）
- 在受控节点上以特权模式运行服务，并在该节点上启动几个容器：一个运行普通 Linux 进程（用于 `os` 注入），一个运行 JVM（用于 `jvm` 注入）。
- 测试流程：
  1. 通过 API 提交对容器的 `cpu`/`network` 等注入（target=os）并校验影响。
  2. 在 JVM 容器內做 jvm 注入（heap/exception），验证可见性与恢复。
  3. 测试 destroy path，确保 cleanup 正常。

注意事项总结
- 运行 server 需要特权，需在部署时严格控制权限与网络访问。
- setns 操作在 Go 中需谨慎处理线程绑定与恢复，建议把执行逻辑限制在短生命周期的线程上。
- 并发 setns 需限流与保护，避免资源耗尽。
- 对于无法进程內执行的场景，提前设计好回退到外部二进制或 agent 模式。

--

我接下来可以：
- A. 基于上述设计，为 `exec/cri` 添加 `ExecInProcess` 设计草案并在代码中实现原型（在 `exec/cri/executor.go` 新增方法和新文件 `exec/cri/ns.go` 的 setns 实现）；或
- B. 先把文档完善后由你审阅。 

请选择下一步（我建议直接实现原型以便验证）。
