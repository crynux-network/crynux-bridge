# Task Lifecycle

This document specifies how Bridge creates, reconciles, and completes inference tasks through Relay.

## Task Creation and Timeout Inputs

Bridge MUST persist each task as an `InferenceTask` before background processing submits it to Relay.

Normal SD and LLM tasks MUST NOT carry a creator-supplied execution timeout. The direct inference-task API MUST reject `timeout` for these task types. The image and OpenAI-compatible LLM APIs MUST NOT expose or forward a timeout. Relay MUST calculate the execution timeout for a normal task after node assignment.

SDFT LoRA tasks MUST retain their Relay timeout input. Bridge MUST reject a request-provided timeout of zero. Bridge MUST store the positive request-provided timeout or `task.sd_finetune_timeout * 60` seconds when the request omits it. Bridge MUST copy that value to validation and retry tasks and include it in every SDFT Relay create request.

## Background Processing

Bridge MUST run the existing `ProcessTasks` loop per `InferenceTask`. The loop MUST load every local task that has not completed locally and MUST ensure that at most one worker processes each local task ID at one time.

Each worker MUST use only the process context. Bridge MUST NOT derive a worker deadline from task creation time or the stored SDFT Relay timeout. A transient database, network, or Relay error MUST cause the worker to retry while the process context remains active.

No Relay-side deadline exists before Relay accepts the create request. If Relay rejects a create request with an HTTP 400 validation response, resubmission can never succeed, and the worker MUST set the task to local `EndAborted` instead of retrying. The worker MUST then update the owning client task.

Bridge MUST NOT change an unfinished task to a local cancellation state because an HTTP request ended. Bridge MUST NOT call Relay's creator-cancel endpoint because an HTTP request ended.

Persisted tasks MUST remain eligible for `ProcessTasks` after restart. A task with a Relay commitment MUST be queried and reconciled before Bridge performs its next stage action.

## HTTP Request Lifetime

Synchronous image and LLM handlers MUST use the incoming HTTP context while waiting for local task completion. Bridge MUST NOT add a fixed three-minute wait deadline.

If the HTTP context is canceled or reaches its caller-provided deadline, the handler MUST return a request error. This event MUST NOT change the stored inference-task status, stop the background worker, or submit cancellation to Relay.

## Relay State Synchronization

Relay task state MUST be authoritative after task submission. Every Relay query result MUST synchronize the local status, `abort_reason`, and `task_error`. Bridge MUST also update the in-memory task values before making the next decision.

If Relay reports that a task Bridge already created does not exist, Relay will never report a status for it again, and Bridge MUST set the task to local `EndAborted`. The not-found response for a task still in local `Pending` keeps its existing meaning: the create request has not succeeded yet and the worker proceeds to create the task.

The Relay terminal statuses visible to Bridge are:

- `TaskEndAborted`
- `TaskEndGroupRefund`
- `TaskEndInvalidated`
- `TaskEndSuccess`
- `TaskEndGroupSuccess`

Bridge MUST map both Relay success statuses to local `EndSuccess`. Bridge MUST download task output only after Relay reports success. Bridge MUST not download output for any Relay abort, including queue timeout, execution timeout, creator-validation timeout, or result-upload timeout.

After a non-success terminal status is synchronized, the worker MUST stop active-stage processing and update the owning client task. After success is synchronized, the worker MUST download the result and persist `ResultDownloaded` before updating the owning client task.

When updating the owning client task after a non-success finished inference task, Bridge MUST inspect every `InferenceTask` with the same `ClientTaskID`. Bridge MUST mark the client task `Failed` only when every such inference task is finished and none has reached `ResultDownloaded`. Bridge MUST NOT mark the client task `Failed` because only the current `TaskID` validation group has finished unsuccessfully while another inference task under the same client task is still unfinished. When any inference task under the client task reaches `ResultDownloaded` while the client task is still `Running`, Bridge MUST mark the client task `Success`.

## Validation Tasks

Bridge MUST persist a task's Relay sequence, sampling seed, VRF proof, and VRF number before submitting validation. While Relay reports a non-terminal status and these values are not persisted, the worker MUST fetch them from Relay and persist them, and MUST create validation members in the same transaction when the VRF selects the task for validation. This applies at every non-terminal Relay status, including `ScoreReady` and `ErrorReported`; reaching a ready status MUST NOT skip this persistence step.

Bridge MUST query Relay while waiting for an LLM node assignment and MUST stop without creating validation members if Relay reports a terminal status.

A task that reaches a terminal status before its VRF data is persisted never gets a validation group. Completion waiters that wait for the persisted VRF data, including the synchronous HTTP handlers and the SDFT client-task processor, MUST stop waiting when the task status is terminal and MUST treat the task as a single-member group.

A task is ready for validation only in `ScoreReady` or `ErrorReported`. Group waiting MUST use explicit ready and terminal status checks and MUST NOT depend on numeric status ordering.

`ScoreReady`, `ErrorReported`, and `EndAborted` members MUST be accepted as group-validation inputs. An `EndAborted` member with queue timeout, execution timeout, or another abort reason MUST NOT prevent Bridge from submitting group validation.

If any validation-group member reaches `EndAborted` with `TaskAbortCreatorValidationTimeout`, Bridge MUST stop waiting for group readiness and MUST NOT submit or retry group validation. Other group members MUST continue to synchronize independently until Relay reports their terminal states.

When a validation-group member's own worker synchronizes that member to `EndAborted`, that worker MUST attempt Relay creator-cancel once for each other group member that has a `TaskIDCommitment`, using `TaskAbortCreatorCancelled`. Bridge MUST NOT query Relay task status before those cancel attempts. Bridge MUST ignore cancel success and failure, including rejection because the sibling is no longer `TaskQueued`, and MUST continue client-task updates. Bridge MUST NOT attempt sibling cancel when the current task is `EndInvalidated`. Bridge MUST NOT attempt sibling cancel only because another group member failed while the current task is still non-terminal.

## Abort Reasons and Legacy Status

Bridge abort-reason values MUST match Relay values from `TaskAbortReasonNone` through `TaskAbortNodeSlashed`. In particular, `TaskAbortCreatorValidationTimeout` MUST be `8` and `TaskAbortResultUploadTimeout` MUST be `9`.

Persisted inference-task status value `12` is reserved and MUST NOT be assigned to new tasks. Migration of a legacy status-12 row MUST follow these rules:

- A row with a Relay task commitment MUST return to `Pending` so `ProcessTasks` queries Relay and reconciles the authoritative state.
- A row without a Relay task commitment MUST become local `EndAborted` with `TaskAbortTimeout`.

## SDFT Completion

The SDFT client-task processor MUST wait using its process context and MUST NOT use the stored Relay timeout as a local wait deadline. A non-final successful checkpoint MUST create a retry inference task with the same stored SDFT timeout. A failed SDFT attempt MUST also preserve that timeout on its retry task.
