# Heartbeat Tasks

Heartbeat tasks are background inference tasks created by Bridge to generate continuous network load.

## Creation Loop

Bridge MUST run a continuous heartbeat creation loop.

When `task.heartbeat_tasks.batch_size` is `0`, Bridge MUST NOT create heartbeat tasks.

When `task.heartbeat_tasks.batch_size` is greater than `0`, each loop iteration MUST:

1. Enforce `max_tasks_per_hour` when it is greater than `0`.
2. Skip creation when the count of local heartbeat tasks in `Pending` or `Started` status is greater than `pending_tasks_limit`.
3. Skip creation when the Relay queued task count is greater than `pending_tasks_limit`.
4. Create up to `batch_size` heartbeat tasks and persist them as local `InferenceTask` records with status `Pending`.

Created heartbeat tasks MUST be submitted to Relay by the shared task processing pipeline.

Each created heartbeat task MUST use:

- `ClientId` `heartbeat-task`
- `TaskSize` `1`
- `Timeout` equal to `timeout_minutes * 60` seconds

## Task Selection

Each heartbeat task entry under `task.heartbeat_tasks.tasks` MUST define:

- `task_version`
- `type` (`sd` or `llm`)
- `ratio`
- `model`
- `min_vram`
- `fee_cnx`
- `timeout_minutes`
- optional `prompts`

Bridge MUST select one eligible task entry by weighted sampling on `ratio`. Entries with `ratio <= 0` MUST be skipped. If no eligible entry exists, task creation MUST fail.

## Prompt Selection

When `prompts` is absent or empty, Bridge MUST use the built-in default prompt for the selected task type and model.

When `prompts` is non-empty, Bridge MUST select exactly one prompt from the list by uniform random selection for each created task.

Each prompt entry MUST define exactly one of:

- `text`
- `content`

`text` and `content` MUST NOT both be set. A prompt with neither `text` nor `content` MUST be rejected at config load.

### SD Prompts

For `type: sd`, each prompt MUST use `text`.

- `text` MUST become the SD task `prompt`
- optional `negative_prompt` MUST become the SD task `negative_prompt`
- `content` MUST be rejected at config load

SD generation parameters other than prompt text (scheduler, steps, cfg, seed, safety checker) MUST follow the existing model-specific heartbeat defaults.

### LLM Prompts

For `type: llm`:

- When `text` is set, Bridge MUST create a single user message whose `content` is that string.
- When `content` is set, Bridge MUST create a single user message whose `content` is the content block list.
- `negative_prompt` MUST be ignored.

Each content block MUST use one of:

- `type: text` with non-empty `text`
- `type: image` with non-empty `base64`

Unsupported content block types MUST be rejected at config load.

For `type: image`, `base64` MUST accept either:

- raw base64 payload
- a `data:*;base64,<payload>` value

Config load MUST normalize image `base64` to raw base64. The final LLM `task_args` image block MUST use:

```json
{ "type": "image", "base64": "<raw-base64>" }
```

The final LLM `task_args` MUST NOT include a data URL prefix in image blocks.

## Task Args Mapping

### SD

Selected SD prompt fields MUST map to Relay SD inference task args:

- `prompt` from selected prompt `text` or built-in default
- `negative_prompt` from selected prompt `negative_prompt` or built-in default
- `base_model.name` from task `model`

### LLM

Selected LLM prompt fields MUST map to Relay GPT inference task args:

- `model` from task `model`
- `messages` as a single user message
- `messages[0].content` as either a string (`text` prompt) or a list of `{type,text|base64}` blocks (`content` prompt)

LLM heartbeat generation config MUST use:

- `max_new_tokens: 250`
- `do_sample: false`
- `temperature: 0`
- `repetition_penalty: 1.1`
- `dtype: bfloat16`
- a random `seed`
