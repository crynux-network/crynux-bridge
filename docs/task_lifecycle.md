# Task Lifecycle

This document specifies how Bridge creates, reconciles, and completes inference tasks through Relay.

## Task Creation and Timeout Inputs

Bridge MUST persist each logical task as a `ClientTask` submission before background processing creates its `InferenceTask` members and submits them to Relay.

Each accepted raw task-creation request MUST create one new `ClientTask`. The raw endpoint MUST NOT accept a request identifier, deduplicate submissions, or return a task created by an earlier submission. The TaskEngine main loop MUST later expand `repeat_num` independent primary tasks in one transaction, and those primary tasks and their protocol validation members MUST share that `ClientTaskID`.

For raw task creation, Bridge MUST commit the conditional API-key usage increment, Client upsert, and `ClientTask` submission in one database transaction. A quota failure, task argument validation failure, task construction failure, persistence failure, or canceled context MUST roll back the usage increment and every created row. Repeat expansion MUST create all primary `InferenceTask` rows and mark expansion complete in a separate all-or-nothing main-loop transaction.

The raw task endpoint MUST authenticate the application API key and MUST require `task_fee` as a base-10 integer string. The value MUST be in the unsigned 256-bit integer range. The submitted value is the final fee in Wei for each repeated primary task. Bridge MUST store it in `inference_tasks.task_fee` as `VARCHAR(78)` and forward it unchanged. Bridge MUST NOT multiply it by task size and MUST NOT read a default fee when the field is absent.

The Chat Completions, Completions, and heartbeat paths MUST read configured CNX fees as ordinary decimal strings with at most 9 fractional digits. Bridge MUST calculate Wei with exact integer arithmetic and MUST reject results outside the unsigned 256-bit integer range. Every `InferenceTask.TaskFee` MUST contain a base-10 Wei string, and Bridge MUST forward that stored value to Relay without another unit conversion. Validation and repeat tasks MUST copy the Wei value unchanged.

Normal SD and LLM tasks MUST NOT carry a creator-supplied execution timeout. The direct inference-task API MUST reject `timeout` for these task types. The OpenAI-compatible LLM APIs MUST NOT expose or forward a timeout. Relay MUST calculate the execution timeout for a normal task after node assignment.

SDFT processing is outside the TaskEngine lifecycle specified by this document. Its existing flow and required component boundary are specified in [sdft_tasks.md](./sdft_tasks.md).

## Background Processing

Bridge MUST process ordinary SD and LLM tasks through the database-driven TaskEngine main loop specified in [task_engine.md](./task_engine.md). The main loop MUST use bounded indexed due-task queries and MUST NOT load every unfinished task or run one long-lived worker per task.

Bounded request workers MUST execute Relay batch creation, batch status, batch validation, batch cancellation, and whole-task result download. A transient database, network, or Relay error MUST be persisted as an operation result, and the main loop MUST schedule the retry through `next_action_at`.

No Relay-side deadline exists before Relay accepts the create request. A permanent per-item create rejection MUST set only that task to local `EndAborted`. A temporary failure or unknown request outcome MUST enter persisted reconciliation and retry without blocking a request worker.

Bridge MUST NOT change an unfinished task to a local cancellation state because an HTTP request ended. Bridge MUST NOT call Relay's creator-cancel endpoint because an HTTP request ended.

Persisted tasks MUST remain eligible for TaskEngine processing after restart. A task with a Relay commitment MUST be queried through batch status and reconciled before Bridge performs its next stage action.

A committed raw task creation MUST remain queryable independently of the HTTP request that created it.

## HTTP Request Lifetime

Synchronous OpenAI-compatible LLM handlers MUST use the incoming HTTP context while polling logical task completion. Bridge MUST NOT add a fixed three-minute wait deadline.

If the HTTP context is canceled or reaches its caller-provided deadline, the handler MUST return a request error. This event MUST NOT change persisted task state, stop TaskEngine processing, or submit cancellation to Relay.

The raw status endpoint MUST call TaskEngine's logical status read method and return immediately. It MUST NOT call a blocking task wait function.

## Relay State Synchronization

Relay task state MUST be authoritative after task submission. Every batch status item MUST be persisted as an operation result. The main loop MUST later apply its status, `abort_reason`, `task_error`, execution assignment, and result availability before making the next decision.

If Relay reports that a task previously confirmed as created no longer exists, Relay will never report a status for it again, and the main loop MUST set the task to local `EndAborted`. A not-found response for a local `Pending` task means that a previous create request did not take effect; the main loop MUST schedule it for a later create batch.

The Relay terminal statuses visible to Bridge are:

- `TaskEndAborted`
- `TaskEndGroupRefund`
- `TaskEndInvalidated`
- `TaskEndSuccess`
- `TaskEndGroupSuccess`

Bridge MUST map both Relay success statuses to local `EndSuccess`. Bridge MUST download the single JSON result for an LLM task or all result images for an SD task only after Relay reports success. Bridge MUST not download output for any Relay abort, including queue timeout, execution timeout, creator-validation timeout, or result-upload timeout.

After a non-success terminal status is synchronized, the main loop MUST stop active-stage processing and recalculate the owning client task. After success is synchronized, the main loop MUST schedule one whole-task result download and persist `ResultDownloaded` only after the complete response is verified and atomically installed.

When recalculating the owning client task after a member finishes, the main loop MUST inspect every `InferenceTask` with the same `ClientTaskID` in one transaction. Bridge MUST mark the client task `Failed` only when every such inference task is finished and none has reached `ResultDownloaded`. Bridge MUST NOT mark the client task `Failed` because only the current `TaskID` validation group has finished unsuccessfully while another inference task under the same client task is still unfinished. When any inference task under the client task reaches `ResultDownloaded` while the client task is still `Running`, Bridge MUST mark the client task `Success`.

TaskEngine status and result reads for ordinary SD and LLM client tasks MUST select one member from the current database snapshot in this order:

1. The earliest `ResultDownloaded` member.
2. An unfinished member when no result has been downloaded.
3. The earliest `EndAborted` or `EndInvalidated` member when all members are finished.
4. The earliest `EndGroupRefund` member when every member has that status.

Within each selection step, Bridge MUST order members by `UpdatedAt ASC, ID ASC`. The selection MUST NOT write task or client-task state.

Raw status and result endpoints MUST authenticate API key format, expiration, and the required admin-or-chat role without applying creation `UseLimit`, then read through TaskEngine. An exhausted key MUST still read and download tasks owned by its client. An invalid or expired key, a key with the wrong role, and access to another client's task MUST be rejected.

## Validation Tasks

Bridge MUST persist a task's Relay sequence, sampling seed, VRF proof, and VRF number before submitting validation. While Relay reports a non-terminal status and these values are not persisted, the main loop MUST apply them from batch status and MUST create validation members in the same transaction when the VRF selects the task for validation. This applies at every non-terminal Relay status, including `ScoreReady` and `ErrorReported`; reaching a ready status MUST NOT skip this persistence step.

Bridge MUST query Relay while waiting for an LLM node assignment and MUST stop without creating validation members if Relay reports a terminal status.

LLM generation is deterministic only within the same GPU variant. A numerical difference produced by another GPU variant can change one generated token, and that token becomes input to the remainder of the generation. Relay compares LLM validation scores by exact equality, so an honest node can be invalidated if the three members use different GPU variants. Before Bridge creates the two additional members of an LLM validation group, it MUST obtain the primary member's selected execution GPU name and GPU VRAM from Relay task status. Both additional members MUST set `RequiredGPU` and `RequiredGPUVram` to that exact pair. Bridge MUST NOT query the mutable node record to obtain these values.

A task that reaches a terminal status before its VRF data is persisted never gets a validation group. Any internal group-state reader that waits for persisted VRF data MUST stop waiting when the task status is terminal and MUST treat the task as a single-member group. OpenAI-compatible handlers MUST read only logical task status and results through TaskEngine and MUST NOT wait for validation-group state.

A task is ready for validation only in `ScoreReady` or `ErrorReported`. Group waiting MUST use explicit ready and terminal status checks and MUST NOT depend on numeric status ordering.

`ScoreReady`, `ErrorReported`, and `EndAborted` members MUST be accepted as group-validation inputs. An `EndAborted` member with queue timeout, execution timeout, or another abort reason MUST NOT prevent Bridge from submitting group validation.

If any validation-group member reaches `EndAborted` with `TaskAbortCreatorValidationTimeout`, Bridge MUST stop waiting for group readiness and MUST NOT submit or retry group validation. Other group members MUST continue to synchronize independently until Relay reports their terminal states.

When the main loop synchronizes a validation-group member to `EndAborted`, it MUST schedule one best-effort cancellation operation for each other group member that has a `TaskIDCommitment`, using `TaskAbortCreatorCancelled`. TaskEngine MUST submit those operations through the Relay cancellation batch endpoint. Bridge MUST treat cancel success, already-cancelled, not-cancellable, not-found, and permanent rejection as completed best-effort outcomes and MUST continue client-task updates. Bridge MUST NOT schedule sibling cancellation when the current task is `EndInvalidated`. Bridge MUST NOT schedule sibling cancellation only because another group member failed while the current task is still non-terminal.

## Abort Reasons and Legacy Status

Bridge abort-reason values MUST match Relay values from `TaskAbortReasonNone` through `TaskAbortNodeSlashed`. In particular, `TaskAbortCreatorValidationTimeout` MUST be `8` and `TaskAbortResultUploadTimeout` MUST be `9`.

Persisted inference-task status value `12` is reserved and MUST NOT be assigned to new tasks. Migration of a legacy status-12 row MUST follow these rules:

- A row with a Relay task commitment MUST return to `Pending` so TaskEngine queries Relay batch status and reconciles the authoritative state.
- A row without a Relay task commitment MUST become local `EndAborted` with `TaskAbortTimeout`.
