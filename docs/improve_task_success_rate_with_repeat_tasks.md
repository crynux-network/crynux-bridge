# Improve Task Success Rate with Repeat Tasks

The Bridge reliability mechanism for task requests MUST create multiple independent primary network tasks for the same API request when `task.repeat_num` is greater than `1`, process those tasks concurrently, and return the first successfully downloaded result to the caller. This behavior MUST increase API-level success probability while keeping request shape and response format unchanged for clients.

## Scope

This document specifies the Bridge-side repeat mechanism used by Bridge task APIs.

The mechanism MUST:

- fan out one API request into multiple primary network tasks;
- run those primary tasks concurrently in background workers;
- return the earliest successful result at API layer;
- keep request ownership and tracking under one Bridge client task record.

The mechanism applies to:

- OpenAI-compatible LLM task APIs;
- SD image generation task APIs.

The SD finetune LoRA API path MUST set `RepeatNum` to `1` explicitly and therefore MUST NOT fan out repeated primary tasks in current implementation.

## Functional Requirements

### Configuration

- The repeat mechanism MUST be controlled by `task.repeat_num`.
- `task.repeat_num = 1` MUST execute one primary task per API request.
- `task.repeat_num > 1` MUST execute that many primary tasks per API request.
- APIs that explicitly set `RepeatNum` in request construction MUST override the config value for that API path.

### Request-to-Task Expansion

- For each API request, Bridge MUST create one `ClientTask`.
- All primary tasks created from that API request MUST reference the same `ClientTaskID`.
- Each primary task MUST be created as an independent network execution attempt.

### Execution and Completion

- Bridge workers MUST process primary tasks concurrently.
- The synchronous API path MUST wait for task completion events and select the first task that reaches result-downloaded state.
- Once the first successful result is available, the API response MUST be built from that task result and returned immediately.
- The completion of other concurrent attempts MUST continue in background and MUST NOT block the API response after the first success is available.

## Implementation Summary

`ProcessGPTTask` and `ProcessSDTask` create tasks, wait for task groups, and then wait for the first downloaded result. `WaitResultTask` races multiple candidate tasks and returns the first result-downloaded task. The worker loop in `ProcessTasks` advances each task through creation, status synchronization, validation signaling, and result download. Request-level ownership is maintained by `ClientTaskID`, which groups all repeated attempts from one API call.

## Data and State Model

### Request-Level Ownership

- `ClientTask` represents one API request lifecycle.
- `InferenceTask.ClientTaskID` represents task membership under that API request.

### Task-Level Lifecycle

Primary tasks and any protocol-required follow-up tasks are processed through task status transitions in background workers until terminal states are reached.

## Source Files

- `api/v1/llm/chat_completions.go`: builds LLM task input and invokes processing.
- `api/v1/llm/completions.go`: builds completion task input and invokes processing.
- `api/v1/image/create_image.go`: builds SD task input and invokes SD processing.
- `api/v1/image/finetune.go`: builds SD finetune task input and explicitly sets `RepeatNum` to `1`.
- `api/v1/inference_tasks/create_task.go`: expands one request into repeated primary tasks based on `repeat_num`.
- `api/v1/inference_tasks/process_task.go`: synchronous processing entrypoint for waiting and returning results.
- `models/inference_task.go`: task grouping/waiting primitives and first-success wait logic.
- `tasks/process_tasks.go`: background task state machine and result download flow.
- `models/client.go`: request-level `ClientTask` model and status tracking.
- `config/app_config.go`: `task.repeat_num` configuration definition.
- `config/config.example.yml`: example configuration value for `repeat_num`.
