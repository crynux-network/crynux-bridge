# Heartbeat Tasks

Heartbeat tasks are background inference tasks created by Bridge to generate continuous network load.

## Creation Loop

Bridge MUST run a continuous heartbeat creation loop.

When `task.heartbeat_tasks.batch_size` is `0`, Bridge MUST NOT create heartbeat tasks.

When `task.heartbeat_tasks.batch_size` is greater than `0`, each loop iteration MUST:

1. Enforce `max_tasks_per_hour` when it is greater than `0`.
2. Select eligible heartbeat task entries by weighted sampling on `ratio`, excluding entries whose in-flight count exceeds `max_pending_tasks`.
3. Create up to `batch_size` heartbeat tasks from eligible entries and persist them as local `InferenceTask` records with status `Pending`.
4. When no eligible entry exists, sleep and continue the loop without creating tasks.

Created heartbeat tasks MUST be submitted to Relay by the shared task processing pipeline.

Each created heartbeat task MUST use:

- `ClientId` `heartbeat-task`
- `TaskSize` `1`

Heartbeat tasks MUST NOT set an execution timeout. Relay MUST calculate the execution timeout after assigning a node.

## Task File Loading

`task.heartbeat_tasks` in `config.yml` MUST contain only control fields and a tasks file pointer:

- `batch_size`
- `max_tasks_per_hour`
- `tasks_file`

When `tasks_file` is omitted or empty, Bridge MUST use `heartbeat_tasks.json`.

`tasks_file` MUST be resolved relative to the directory that contains the loaded `config.yml`. An absolute `tasks_file` path MUST also be accepted.

At config load, Bridge MUST read that JSON file and populate `task.heartbeat_tasks.tasks` from the top-level `tasks` array. Bridge MUST reject startup when the file cannot be read or parsed.

Deployment MUST place the tasks JSON file next to `config.yml`. Image files referenced by `image_path` MUST use the same resolution rule.

## Task Selection

Each heartbeat task entry under `task.heartbeat_tasks.tasks` MUST define:

- `task_version`
- `type` (`sd` or `llm`)
- `ratio`
- `model`
- `min_vram`
- `fee_cnx`
- `max_pending_tasks` when `ratio > 0`
- `max_new_tokens` when `type` is `llm`
- optional `prompts`

When `ratio > 0`, `max_pending_tasks` MUST be greater than `0`. Config load MUST reject entries that violate this rule.

When `type` is `llm`, `max_new_tokens` MUST be greater than `0`. Config load MUST reject LLM entries that violate this rule.

When `type` is `sd`, `max_new_tokens` MUST NOT be set. Config load MUST reject SD entries that set `max_new_tokens`.

Bridge MUST select one eligible task entry by weighted sampling on `ratio`. Entries with `ratio <= 0` MUST be skipped.

In-flight counting MUST use local heartbeat tasks whose `ClientId` is `heartbeat-task` and whose status is not terminal.

Terminal statuses MUST be: `EndAborted`, `EndGroupRefund`, `EndInvalidated`, `EndSuccess`, and `ResultDownloaded`.

`max_pending_tasks` MUST limit how many unfinished heartbeat tasks of that entry remain in Bridge and Relay. Tasks that have already been submitted to Relay and are still unfinished MUST count toward the limit.

In-flight counts MUST be keyed by `task_type` and the entry `model`. The stored `task_model_ids` value MUST match the base model id produced by `GetTaskConfigModelIDs` for that entry. Entries that share the same `type` and `model` MUST share one in-flight count pool.

An entry MUST be excluded from sampling when its in-flight count is greater than its `max_pending_tasks`. An entry whose in-flight count equals `max_pending_tasks` MUST remain eligible.

Within a single batch, Bridge MUST accumulate in-memory increments per `type` and `model` so later samples in the same batch observe tasks already chosen earlier in that batch.

When no eligible entry remains, task creation for that loop iteration MUST stop without error.

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
- `type: image` with exactly one of `base64` or `image_path`

Unsupported content block types MUST be rejected at config load.

For `type: image`:

- `base64` and `image_path` MUST be mutually exclusive
- `image_path` MUST be resolved relative to the directory that contains the loaded `config.yml`; an absolute path MUST also be accepted
- when `image_path` is set, config load MUST read the file bytes, encode them as raw base64 into `base64`, and clear `image_path` before validation
- when `base64` is set, it MUST accept either a raw base64 payload or a `data:*;base64,<payload>` value

Config load MUST normalize image `base64` to raw base64. The final LLM `task_args` image block MUST use:

```json
{ "type": "image", "base64": "<raw-base64>" }
```

The final LLM `task_args` MUST NOT include a data URL prefix in image blocks.
The final LLM `task_args` MUST NOT include `image_path`.

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

- `max_new_tokens` from the selected task entry `max_new_tokens`
- `do_sample: false`
- `temperature: 0`
- `repetition_penalty: 1.1`
- `dtype: bfloat16`
- a random `seed`
