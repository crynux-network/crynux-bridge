# Relay Timeout 适配 Review 与修复记录

## 目的

本文记录 Bridge 适配 Relay 所有权超时机制时发现的问题、根因、修复要求和后续 Review 检查方法。

Relay 的接口校验和状态处理是本次适配的依据。Review Bridge 改动时，必须同时检查 Relay 对应版本的创建接口、状态枚举和验证服务，不能只根据 Bridge 当前逻辑推断 Relay 行为。

## 问题一：普通超时成员被错误地排除在组验证之外

### Relay 行为

Relay 允许以下状态的成员进入组验证：

- `TaskScoreReady`
- `TaskErrorReported`
- `TaskEndAborted`

只有状态为 `TaskEndAborted` 且原因为 `TaskAbortCreatorValidationTimeout` 的成员必须永久阻止组验证。

普通队列超时、执行超时等 `TaskEndAborted` 成员仍然必须作为组验证输入。Relay 会根据剩余有效结果数量和结果是否一致决定成功、退款或 `TaskAbortGroupTimeout`。

### 错误实现

Bridge 曾要求三个成员全部为 `ScoreReady` 或 `ErrorReported` 才提交组验证。这样会把普通 `EndAborted` 成员错误地视为不可验证。

结果是：

1. 其他成员已经提交结果。
2. Bridge 不调用 Relay 组验证。
3. 其他成员继续等待。
4. 其他成员最终进入 `TaskAbortCreatorValidationTimeout`。
5. 任务失败，并错误地按照 Creator 未及时验证进行结算。

### 修复要求

Bridge 判断组验证输入资格时必须接受：

```go
status == InferenceTaskScoreReady ||
status == InferenceTaskErrorReported ||
status == InferenceTaskEndAborted
```

Bridge 必须单独检查：

```go
status == InferenceTaskEndAborted &&
abortReason == TaskAbortCreatorValidationTimeout
```

该检查结果为真时，Bridge 必须停止提交和重试组验证。

状态资格和终止原因必须分开判断。不能用“所有终态都阻止验证”代替 Relay 的实际规则。

### 必须覆盖的测试

- 两个成员已提交结果，一个成员普通超时：允许组验证。
- 一个成员为 `TaskAbortCreatorValidationTimeout`：禁止组验证。
- 三个成员均为 `ScoreReady` 或 `ErrorReported`：允许组验证。
- 存在 Relay 不接受的其他状态：禁止组验证。

## 问题二：SDFT 验证子任务没有复制 Timeout

### Relay 行为

Relay 要求每个 SDFT 创建请求的 `timeout` 大于零。该要求适用于：

- 原始 SDFT 任务
- VRF 选中的验证子任务
- 检查点续跑任务
- 失败重试任务

### 错误实现

Bridge 创建两个验证子任务时复制了任务参数、费用、GPU 要求和 VRF 信息，但没有复制原始任务的 `Timeout`。

验证子任务因此携带 `timeout=0`，Relay 拒绝创建。Bridge 随后持续重试，原始任务最终进入 Creator Validation Timeout。

### 修复要求

所有从现有 SDFT `InferenceTask` 派生的新任务都必须复制：

```go
Timeout: task.Timeout
```

创建验证子任务的逻辑必须集中在一个构造函数中，避免以后新增字段时只修改部分创建路径。

### 必须覆盖的测试

- SDFT 验证子任务数量为两个。
- 两个验证子任务的 `Timeout` 都等于原始任务的 `Timeout`。
- SDFT 检查点续跑任务保留 `Timeout`。
- SDFT 失败重试任务保留 `Timeout`。

## 问题三：Bridge 接受显式 timeout=0 的 SDFT 请求

### Relay 行为

Relay 对 SDFT 创建请求要求：

```go
timeout > 0
```

### 错误实现

Bridge 只判断请求是否提供了 `timeout` 指针。显式传入零值时，Bridge 将零值保存到本地并返回创建成功。

后台提交 Relay 时请求必然失败，并会持续重试。

### 修复要求

Bridge 必须在保存任务之前完成以下校验：

- 显式提供的 SDFT `timeout` 必须大于零。
- 请求未提供 `timeout` 时，`task.sd_finetune_timeout * 60` 的结果必须大于零。
- 普通 SD 和 LLM 任务必须拒绝直接提供 `timeout`。

无效输入必须作为 `timeout` 字段的参数校验错误返回，不能先保存任务再等待后台失败。

### 必须覆盖的测试

- SDFT 显式正数 timeout：接受并保留原值。
- SDFT 未提供 timeout：使用配置值。
- SDFT 显式 timeout 为零：拒绝。
- SDFT 默认配置为零：拒绝。
- 普通 SD 或 LLM 提供 timeout：拒绝。
- 普通 SD 或 LLM 未提供 timeout：本地值为零，Relay 请求不包含该字段。

## 问题四：任务先到 ScoreReady 时 VRF 数据持久化被跳过

### Relay 行为

Relay 的验证接口要求 Creator 提交基于任务 `SamplingSeed` 计算的 VRF proof。VRF proof 缺失或错误时，验证请求必然被拒绝。

Creator 在任务到达 `ScoreReady` 或 `ErrorReported` 之后补算 VRF、补建验证子任务是协议允许的，只是会消耗 `ScoreReadyTime` 起算的 creator-validation 时限。

Creator 一直不完成验证时，Relay 在 creator-validation 超时后把任务置为 `TaskEndAborted`，原因为 `TaskAbortCreatorValidationTimeout`，任务费全额按成功任务的分成规则支付，由 Creator 承担。

### 错误实现

Bridge 把"同步 Sequence、持久化 SamplingSeed/VRFProof/VRFNumber、创建验证子任务"这一步的执行条件写成了"任务状态属于 Created、Started、ParamsUploaded"。同时在这段代码内部（包括 LLM 等待节点分配的轮询循环）加入了 `goto validation`：一旦观察到状态越过 before-validation，就直接跳到验证阶段。

只要任务在 Bridge 持久化 VRF 数据之前先到达了 `ScoreReady` 或 `ErrorReported`（典型场景：Bridge 与 Relay 之间网络中断，worker 报错重试后重新进入处理函数时任务已执行完），VRF 数据就永远不会被持久化：

1. 验证阶段查到的任务组只有原始任务一个成员。
2. Bridge 用空的 VRF proof 调用单任务验证，Relay 拒绝。
3. worker 重试时任务状态已是 `ScoreReady`，持久化代码被条件跳过，形成死循环。
4. 任务最终以 `TaskAbortCreatorValidationTimeout` 终止，Bridge 支付全额任务费，client task 失败。
5. 同步 API 的等待方在 `WaitTaskGroup` 中等待 `VRFNumber` 非空，会一直等到 HTTP 请求结束。

### 修复要求

持久化这一步的执行条件必须改为数据本身的状态，不能依赖任务的协议阶段：

```go
!isRelayTerminalTaskStatus(task.Status) && (task.Sequence == 0 || len(task.VRFNumber) == 0)
```

即：只要 Relay 未报告终态，且本地还没有持久化 Sequence 或 VRF 数据，就必须执行这一步。任务已经到达 `ScoreReady` 或 `ErrorReported` 时同样执行。

这段代码内部不允许因为状态前进而跳过持久化。LLM 等待节点分配的循环只在 Relay 终态时提前退出；状态到达 `ScoreReady` 时 `SelectedNode` 必然已存在，循环通过 SelectedNode 检查自然结束。

验证子任务（`SamplingSeed` 非空）只需要补持久化 `Sequence`，不重复计算 VRF。

### 必须覆盖的测试

- `ScoreReady` 且 VRF 数据未持久化：必须执行持久化步骤。
- 验证子任务 `Sequence` 为零：必须执行持久化步骤。
- Sequence 和 VRF 数据均已持久化：不得重复执行。

## 问题五：任务在 VRF 数据持久化之前终止时，等待方永远等不到结束

### Relay 行为

任务在 Relay 创建成功后，队列超时和执行超时的 deadline 由 Relay 拥有。Relay 可以在 Bridge 持久化 Sequence/VRF 数据之前把任务置为 `TaskEndAborted`（典型场景：创建成功后 Bridge 与 Relay 之间网络中断，恢复时任务已是终态）。

按照设计，Bridge 的 worker 在 Relay 报告终态后不再持久化 VRF 数据。因此"终态 + VRF 数据永远为空"是一个合法的、永久的最终状态。

### 错误实现

`models.WaitTaskGroup` 只有一个退出条件：`VRFNumber` 非空。它是 Bridge 内部所有"等待任务完成"路径的入口：

1. 同步 HTTP 处理器（`ProcessGPTTask`/`ProcessSDTask`）：请求会一直挂着，直到调用方自己的 HTTP 超时。本地 3 分钟兜底 deadline 已被移除，调用方不设超时就永远挂着。
2. SDFT 的 ClientTask 处理器（`processSDFTTask`）：传入的是进程级 background context，永远不会取消。该路径永久挂死，既不创建重试任务，也不把 ClientTask 置为 Failed，ClientTask 永远 Running。

### 修复要求

`WaitTaskGroup` 在 `VRFNumber` 为空且任务状态已是 Relay 终态时必须立即返回，把该任务作为单成员组返回：

```go
if IsRelayTerminalTaskStatus(task.Status) {
    return []InferenceTask{*task}, nil
}
```

后续 `WaitResultTask` 对终态无结果的任务返回 `ErrTaskEndWithoutResult`，同步 API 返回错误，SDFT 走失败重试分支。

Relay 终态判断提取为 `models.IsRelayTerminalTaskStatus`，`tasks` 包复用同一个函数，不允许各处维护重复的终态列表。

### 必须覆盖的测试

- 任务为 `EndAborted` 且 VRF 数据为空：`WaitTaskGroup` 立即返回单成员组。
- Relay 终态列表与非终态列表逐个状态断言。

## 问题六：Relay 永久拒绝的请求没有终结路径，会无限重试

### Relay 行为

Relay 的 deadline 从任务创建成功才开始。任务创建成功之前，整个网络里没有任何组件为这个阶段持有 deadline。

Relay 对创建请求本身的校验失败返回 HTTP 400（余额不足、task fee 过低、参数校验失败等）。同一个请求重试永远不会成功。

Relay 的历史清理会删除已终结的任务。之后查询该任务返回 "Task not found"，Relay 永远不会再报告它的任何状态。

### 错误实现

Bridge 的 worker 对 `createTask` 的所有失败一律无限重试（每 2–5 秒一次）。对 400 拒绝：

1. InferenceTask 永远非终态，ClientTask 永远 Running。
2. Heartbeat 任务永久占用 `max_pending_tasks` 配额。
3. 任务永远留在 `getUnprocessedTasks` 里，每个挂一个 goroutine。

对已创建任务的 "Task not found"（Bridge 长时间停机期间任务在 Relay 终结并被历史清理删除），`syncTask` 同样报错并无限重试。

### 修复要求

创建请求收到 HTTP 400（`RelayError.StatusCode == 400`）时，worker 必须把任务置为本地 `EndAborted`，记录 trace 事件，并立即更新所属 ClientTask，不允许重试。5xx 和网络错误仍然重试。

`syncTask` 收到 "Task not found" 时按本地状态区分：

- 本地状态为 `Pending`：保持原语义，任务尚未创建成功，继续走创建流程。
- 本地状态为已创建：Relay 已删除该任务，必须置为本地 `EndAborted`，由调用方的终态分支更新 ClientTask。

### 必须覆盖的测试

- `RelayError` 400：判定为永久拒绝（含 wrap 后的错误）。
- `RelayError` 5xx：判定为可重试。
- 非 `RelayError` 的网络错误：判定为可重试。
- "Task not found" 消息：正确识别；其他 400 错误消息不得误判为 not-found。

## 后续 Review 检查顺序

### 1. 先读取 Relay 的输入校验

检查 Relay 创建接口对每种任务类型的必填字段、零值规则和忽略规则。Bridge 必须在本地保存前拒绝 Relay 必然拒绝的请求。

### 2. 列出所有任务创建路径

修改任务字段时必须逐项检查：

- API 原始任务
- Heartbeat 任务
- VRF 验证子任务
- SDFT 检查点续跑任务
- SDFT 失败重试任务

不能只检查 API 创建路径。

### 3. 同时检查状态和原因

`EndAborted` 不能被当成单一业务结果。Review 时必须检查 `AbortReason`。

特别是：

- 普通 Queue 或 Execution Timeout 可以进入组验证。
- Creator Validation Timeout 必须阻止组验证。
- Result Upload Timeout 是终态，不能下载结果。

### 4. 对照 Relay 的完整状态条件

Bridge 的条件必须逐项对应 Relay 的条件。不能使用枚举大小比较，也不能把多个不同状态概括成更严格的本地条件。

### 5. 测试失败分支

每个字段或状态修改至少测试：

- 正常值
- 零值
- 缺失值
- 派生任务复制
- 普通超时
- Creator Validation Timeout
- Relay 终态

### 6. 检查每个可持久化字段在所有可达状态下都会被写入

字段的写入条件必须基于"该字段是否已经写入"，不能基于"任务当前处于哪个协议阶段"。状态可以在 Bridge 的两次观察之间跨越多个阶段，任何依赖中间阶段的写入逻辑都会在网络中断和重试路径下被跳过。

## 本次修复涉及的主要文件

- `api/v1/inference_tasks/create_task.go`
- `api/v1/inference_tasks/create_task_test.go`
- `models/inference_task.go`
- `models/inference_task_test.go`
- `tasks/process_tasks.go`
- `tasks/process_tasks_timeout_test.go`
- `docs/task_lifecycle.md`
