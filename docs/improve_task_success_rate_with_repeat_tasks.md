# Improve Task Success Rate with Repeat Tasks

The Bridge reliability mechanism MUST create multiple independent primary network tasks for one logical task submission when its selected `repeat_num` is greater than `1`, process those tasks concurrently, and return the first successfully downloaded result to the caller.

## Scope

This document specifies the Bridge-side repeat mechanism used by TaskEngine.

The mechanism MUST:

- fan out one logical task submission into multiple primary network tasks;
- run those primary tasks concurrently in background workers;
- return the earliest successful result at API layer;
- keep request ownership and tracking under one Bridge client task record.

This mechanism is Bridge reliability fan-out. It MUST NOT be described or implemented as task validation. It MUST NOT replace, trigger, or configure VSS or single-task validation. It MUST NOT decide whether validation runs, how many validation members exist, or how scores are compared.

Each repeat primary has a distinct `TaskID`. Protocol validation members for one primary share that primary's `TaskID`. Those are separate mechanisms: repeat expansion creates independent primaries under one `ClientTaskID`; VSS expansion, when required, creates validation members under one primary `TaskID`.

Raw task and OpenAI-compatible submissions MUST use this mechanism with the configured `task.repeat_num`. Heartbeat submissions MUST explicitly set `repeat_num` to `1` on the TaskEngine submission and therefore MUST create one primary task. SDFT round creation MUST use `repeat_num` equal to `1`.

## Functional Requirements

### Configuration

- The effective `repeat_num` MUST come from `task.repeat_num` in Bridge configuration when the TaskEngine submission does not override it.
- TaskEngine's logical-task submission MAY contain an optional `repeat_num` for Bridge-internal callers.
- When the submission provides `repeat_num`, TaskEngine MUST use that value.
- When the submission omits `repeat_num`, TaskEngine MUST use `task.repeat_num`.
- The selected value MUST be greater than `0`.
- A selected value of `1` MUST create one primary task.
- A selected value greater than `1` MUST create that many primary tasks.

### Request-to-Task Expansion

- For each logical task submission, TaskEngine MUST create one `ClientTask`.
- All primary tasks created from that submission MUST reference the same `ClientTaskID`.
- Each primary task MUST be created as an independent network execution attempt.

### Execution and Completion

- Bridge workers MUST process primary tasks concurrently.
- The synchronous API path MUST poll logical task status through the same TaskEngine status method used by the raw task API.
- TaskEngine MUST NOT provide a completion waiter or task-completion notification for synchronous APIs.
- Once the first successful result is available, the API response MUST be built from that task result and returned immediately.
- The completion of other concurrent attempts MUST continue in background and MUST NOT block the API response after the first success is available.
- Client-task failure status MUST be decided from all inference tasks under the same `ClientTaskID`, not from a single primary or validation group alone. One unsuccessful primary or validation group MUST NOT mark the client task failed while another attempt under the same client task is still unfinished.

## Implementation Summary

TaskEngine's main loop advances every primary task through creation, status synchronization, validation, and result download. The first downloaded result updates the logical task to success. A synchronous API handler observes that state by polling TaskEngine's logical status method and then reads the result through TaskEngine's logical result method. Request-level ownership is maintained by `ClientTaskID`, which groups all repeated attempts from one logical submission.

## Data and State Model

### Request-Level Ownership

- `ClientTask` represents one API request lifecycle.
- `InferenceTask.ClientTaskID` represents task membership under that API request.

### Task-Level Lifecycle

Primary tasks and any protocol-required follow-up tasks are processed through task status transitions in background workers until terminal states are reached.

## Source Files

- `api/v1/llm/chat_completions.go`: builds LLM task input and invokes processing.
- `api/v1/llm/completions.go`: builds completion task input and invokes processing.
- `api/v1/inference_tasks/create_task.go`: normalizes raw and OpenAI-compatible task submissions.
- OpenAI-compatible handlers: API-layer status polling and response conversion.
- `taskengine/`: logical task submission, repeat expansion, shared status and result reads, background state processing, and result download.
- `tasks/process_tasks.go`: SDFT-only background processing.
- `models/client.go`: request-level `ClientTask` model and status tracking.
- `config/app_config.go`: `task.repeat_num` configuration definition.
- `config/config.example.yml`: example configuration value for `repeat_num`.
