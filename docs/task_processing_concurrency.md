# Task Processing Concurrency

This document specifies how Bridge bounds task-processing concurrency and prevents conflicting lifecycle writes. The complete state machine and interface contract are specified in [task_engine.md](./task_engine.md).

## State Ownership

One Bridge database MUST be processed by exactly one TaskEngine instance. TaskEngine MUST run one main loop, and only that loop may:

- apply Relay operation results to `InferenceTask` lifecycle state;
- create repeat and VSS members;
- decide VSS group readiness and validation ownership;
- update `ClientTask.Status` and `ClientTask.FailedCount`;
- calculate retry time and `next_action_at`.

Request workers MUST NOT write task lifecycle, VSS group, or client-task state. They MUST write only the persisted result of their assigned external operation. This separation MUST prevent a stale Relay response from replacing a newer local state such as `ResultDownloaded`.

## Bounded External Requests

TaskEngine MUST use separately bounded request workers for Relay batch creation, batch status, batch validation, batch cancellation, and whole-task result download.

The main loop MUST group due create, status, validation, and cancellation operations by creator and operation type, split them by configured batch limits, persist every included item as running, and submit one HTTP request job per batch. It MUST NOT create one long-running goroutine per task or wait beyond the normal scan iteration to fill a batch.

One task MUST have at most one running external operation. A request timeout, response loss, process interruption, or whole-request error MUST leave mutation results unknown until TaskEngine reconciles every affected commitment through batch status. TaskEngine MUST retry only operations that Relay confirms did not take effect and still remain required.

A batch mutation MUST allow partial success. The request worker MUST persist one outcome per task or validation unit, and the main loop MUST apply those outcomes independently. One item failure MUST NOT roll back, retry, or locally fail another successful item.

## Database Scheduling

The main loop MUST select only bounded due batches through the indexes defined in `task_engine.md`. It MUST NOT load every unfinished task or use `OFFSET` pagination over a changing unfinished set.

Waiting for Relay progress and retry backoff MUST be represented by `next_action_at`. No task-processing function may hold a goroutine while sleeping for a later lifecycle stage.

## Restart Recovery

TaskEngine MUST treat every persisted running operation as interrupted after restart. It MUST reconcile interrupted mutations with Relay before retrying them and MUST verify or discard incomplete LLM result files and SD result archives before another download.

TaskEngine MUST recalculate every `Running` ClientTask whose members may already be finished. A process exit between member completion and client-task aggregation MUST therefore not leave the client task permanently running.

## Capacity Verification

Load tests MUST cover batch item partial success, unknown mutation reconciliation, VSS groups, repeats, whole-task result downloads, and restart recovery. They MUST verify configured request concurrency limits, bounded goroutine count, bounded due-query sizes, stable database and ledger backlogs, and Relay request count and items per request at the target sustained task rate.
