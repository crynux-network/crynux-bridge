# Heartbeat Tasks

Heartbeat tasks are background inference tasks created by Bridge to generate continuous network load.

## Creation Loop

Bridge MUST run a continuous heartbeat creation loop.

When `task.heartbeat_tasks.batch_size` is `0`, Bridge MUST NOT create heartbeat tasks.

When `task.heartbeat_tasks.batch_size` is greater than `0`, each loop iteration MUST:

1. Enforce `max_tasks_per_hour` when it is greater than `0`.
2. Load per-slot in-flight counts and enforce `max_total_pending_tasks` when it is greater than `0`.
3. Select eligible heartbeat task entries by weighted sampling on `ratio`, excluding entries whose in-flight count exceeds `max_pending_tasks`.
4. Convert up to `batch_size` selected entries into normalized logical task submissions and call `TaskEngine.CreateTask` for each submission with `repeat_num` equal to `1`.
5. When no eligible entry exists, sleep and continue the loop without creating tasks.

When `max_total_pending_tasks` is greater than `0`, Bridge MUST sum all per-slot in-flight counts for client `heartbeat-task`. When that sum is greater than or equal to `max_total_pending_tasks`, Bridge MUST create no heartbeat tasks in that loop iteration. When the sum is below the limit, Bridge MUST shrink that iteration's create count so the sum of already in-flight tasks and newly created tasks does not exceed `max_total_pending_tasks`. When `max_total_pending_tasks` is `0`, Bridge MUST NOT apply a cross-slot in-flight limit.

The heartbeat creation loop MUST NOT create or process `ClientTask` or `InferenceTask` records directly. After it calls `TaskEngine.CreateTask`, the submitted task MUST use the standard `ClientTask` persistence, repeat expansion, `InferenceTask` creation, and Relay processing flow.

Each created heartbeat task MUST use:

- `ClientId` `heartbeat-task`
- `TaskSize` `1`

Heartbeat tasks MUST NOT set an execution timeout. Relay MUST calculate the execution timeout after assigning a node.

## Queued Wait Limit

`task.heartbeat_tasks.max_queued_duration` MUST be a non-negative duration. When the value is `0`, Bridge MUST NOT apply a heartbeat-only queued wait limit.

When `max_queued_duration` is greater than `0`, TaskEngine MUST enforce the limit only for client `heartbeat-task`. During Relay status synchronization, when Relay reports `TaskQueued` and `now - InferenceTask.CreatedAt` is greater than or equal to `max_queued_duration`, TaskEngine MUST in the same transaction:

1. Set the local inference task to `EndAborted` with `TaskAbortCreatorCancelled`.
2. When the task has a `TaskIDCommitment`, set `sibling_cancellation_required` so the existing Relay batch cancellation path submits `TaskAbortCreatorCancelled`.

The local `EndAborted` transition MUST complete whether or not Relay cancellation succeeds. Cancel outcomes `cancelled`, `already_cancelled`, `not_cancellable`, `not_found`, and permanent rejection MUST clear `sibling_cancellation_required` and MUST leave the local task `EndAborted` with `TaskAbortCreatorCancelled`. A temporary cancellation transport failure MUST keep `sibling_cancellation_required` and retry cancellation without restoring a non-terminal local status.

While a heartbeat task is locally `EndAborted` with `sibling_cancellation_required` still set, a later status sync that still reports a non-terminal Relay status MUST NOT overwrite the local `EndAborted` status or clear `sibling_cancellation_required`.

After the local task reaches `EndAborted`, it MUST NOT count toward heartbeat in-flight totals used by `max_pending_tasks` and `max_total_pending_tasks`.

This limit MUST NOT apply to raw or API client tasks. Bridge MUST NOT force-abort a heartbeat task under this rule when Relay reports a status other than `TaskQueued`.

## Task File Loading

`task.heartbeat_tasks` in `config.yml` MUST contain only control fields and a tasks file pointer:

- `batch_size`
- `max_tasks_per_hour`
- `max_total_pending_tasks`
- `max_queued_duration`
- `tasks_file`

When `tasks_file` is omitted or empty, Bridge MUST use `heartbeat_tasks.json`.

`tasks_file` MUST be resolved relative to the directory that contains the loaded `config.yml`. An absolute `tasks_file` path MUST also be accepted.

At config load, Bridge MUST read that JSON file and populate `task.heartbeat_tasks.tasks` from the top-level `tasks` array. Bridge MUST reject startup when the file cannot be read or parsed.

Deployment MUST place the tasks JSON file next to `config.yml`. Image files referenced by `image_path` MUST use the same resolution rule.

## Task Selection

Each heartbeat task entry under `task.heartbeat_tasks.tasks` MUST define:

- `task_version`, which Bridge MUST write into TaskArgs.`version` for both SD and LLM heartbeat TaskArgs
- `type` (`sd` or `llm`)
- `ratio`
- `model`
- `min_vram`
- `fee_cnx`
- `max_pending_tasks` when `ratio > 0`
- `max_new_tokens` when `type` is `llm`
- `steps` when `type` is `sd`
- optional `dtype` when `type` is `sd`
- optional `quantization_config` when `type` is `sd` or `llm`
- optional `components` when `type` is `sd`
- optional `tools` when `type` is `llm`
- optional `tool_choice` when `type` is `llm`
- optional `response_format` when `type` is `llm`
- optional `prompts`

When `ratio > 0`, `max_pending_tasks` MUST be greater than `0`. Config load MUST reject entries that violate this rule.

When `type` is `llm`, `max_new_tokens` MUST be greater than `0`. Config load MUST reject LLM entries that violate this rule.

When `type` is `sd`, `steps` MUST be greater than `0`. Config load MUST reject SD entries that violate this rule.

When `type` is `sd`, `max_new_tokens` MUST NOT be set. Config load MUST reject SD entries that set `max_new_tokens`.

When `type` is `llm`, `steps` MUST NOT be set. Config load MUST reject LLM entries that set `steps`.

When `type` is `llm`, `dtype` MUST NOT be set. Config load MUST reject LLM entries that set `dtype`. LLM heartbeat TaskArgs MUST always use `dtype: bfloat16`.

When `type` is `sd`, `tools` MUST NOT be set. Config load MUST reject SD entries that set `tools`.

When `type` is `sd`, `tool_choice` MUST NOT be set. Config load MUST reject SD entries that set `tool_choice`.

When `type` is `sd`, `response_format` MUST NOT be set. Config load MUST reject SD entries that set `response_format`.

When `type` is `llm`, `components` MUST NOT be set. Config load MUST reject LLM entries that set `components`.

When `quantization_config` is present and non-empty, `quantization_config.quant_method` MUST be a non-empty string. Config load MUST reject entries that violate this rule.

When `components` is present and non-empty, it MUST be a JSON object. Config load MUST reject entries that violate this rule.

`quantization_config`, `components`, `tools`, `tool_choice`, and `response_format` MUST be copied into TaskArgs as the exact JSON text from `heartbeat_tasks.json`. Bridge MUST NOT decode those objects into generic maps and re-encode them in a way that changes number literals. When a field inside those objects must be a Python `float` on the worker (for example `quant_kwargs.llm_int8_threshold`), the heartbeat tasks file MUST write that value as a JSON number with a fractional part (for example `6.0`, not `6`). Repository name casing in `model` and `components.*.repo` is not a correctness requirement of the heartbeat config: `TaskEngine.CreateTask` MUST normalize Hugging Face model name fields before persisting the submission.

When any prompt under an LLM entry contains assistant `tool_calls`, that entry's `tools` MUST be a non-empty array. Config load MUST reject entries that violate this rule.

When `tool_choice` is set on an LLM entry, it MUST be one of:

- the string `none`
- the string `auto`
- the string `required`
- an object `{"type":"function","function":{"name":"<tool-name>"}}`

When `tool_choice` is `auto`, `required`, or a named function object, that entry's `tools` MUST be a non-empty array. Config load MUST reject entries that violate this rule.

When `tool_choice` is a named function object, `function.name` MUST equal the `function.name` of exactly one entry in that task's `tools`. Config load MUST reject entries that violate this rule.

When `response_format` is set on an LLM entry, it MUST be one of:

- `{"type":"json_object"}`
- `{"type":"json_schema","json_schema":{"name":"<name>","schema":{...}}}`

For `json_schema`, `json_schema.name` MUST be a non-empty string and `json_schema.schema` MUST be an object. Config load MUST reject entries that violate this rule.

An LLM entry MAY set both `tool_choice` and `response_format`.

Bridge MUST select one eligible task entry by weighted sampling on `ratio`. Entries with `ratio <= 0` MUST be skipped.

In-flight counting MUST include both unexpanded heartbeat `ClientTask` submissions and expanded heartbeat `InferenceTask` records whose status is not terminal.

Terminal statuses MUST be: `EndAborted`, `EndGroupRefund`, `EndInvalidated`, `EndSuccess`, and `ResultDownloaded`.

`max_pending_tasks` MUST limit how many unfinished heartbeat tasks of that entry remain in Bridge and Relay. A committed `ClientTask` MUST count before repeat expansion. Expansion MUST atomically replace that logical submission count with its created `InferenceTask` count so the same heartbeat task is not counted twice.

In-flight counts MUST be keyed by `task_type`, the entry `model`, and the entry's quantization identity. When `components` is absent or empty, the quantization identity MUST be the lowercase hexadecimal SHA-256 of the canonical JSON of the complete `quantization_config`, and when `quantization_config` is also absent or empty Bridge MUST hash the canonical JSON `null`. When `components` is present and non-empty, the quantization identity MUST be the lowercase hexadecimal SHA-256 of the canonical JSON of the object `{"quantization_config": <decoded or null>, "components": <decoded>}`. An unexpanded `ClientTask` MUST persist indexed submission task type, model-id, and quantization-identity fields. The stored submission model id and expanded `task_model_ids` value MUST match the base model id produced by `GetTaskConfigModelIDs` for that entry. Entries that share the same `type`, `model`, and quantization identity MUST share one in-flight count pool. Entries that differ only in quantization identity MUST use separate in-flight count pools.

An entry MUST be excluded from sampling when its in-flight count is greater than its `max_pending_tasks`. An entry whose in-flight count equals `max_pending_tasks` MUST remain eligible.

Within a single batch, Bridge MUST accumulate in-memory increments per `type`, `model`, and quantization identity so later samples in the same batch observe tasks already chosen earlier in that batch.

When no eligible entry remains, task creation for that loop iteration MUST stop without error.

## Prompt Selection

When `prompts` is absent or empty, Bridge MUST use the built-in default prompt for the selected task type and model.

When `prompts` is non-empty, Bridge MUST select exactly one prompt from the list by uniform random selection for each created task.

When `type` is `llm` and `ratio > 0`, each prompt MUST contain at least one `{{hb_rand}}` token in `text`, a `type: text` content block, or a string `content` on a message. Config load MUST reject those prompts that omit the token.

When creating an LLM heartbeat task, Bridge MUST copy the selected prompt, generate a 16-character lowercase hex value with `crypto/rand`, and replace every `{{hb_rand}}` in that copy. Replacement MUST cover `text`, `type: text` content blocks, and every string `content` on `messages`, including tool messages when the configured string contains the token. Bridge MUST NOT mutate the in-memory config prompt. After replacement, the 16-character hex value MUST appear in the prompt. If the selected LLM prompt contains no `{{hb_rand}}`, Bridge MUST fail that task creation and log the error.

The built-in default LLM prompt MUST contain `{{hb_rand}}`. Bridge MUST apply the same replacement to that default prompt.

`{{hb_rand}}` MUST be used as a value inside the task request, such as a ticket number, a query string, or a branch condition. LLM prompts MUST NOT append a labeled side field such as `Request id:` that is unrelated to the request body.

When `type` is `sd`, Bridge MUST NOT replace `{{hb_rand}}`.

Each prompt entry MUST define exactly one of:

- `text`
- `content`
- `messages`

`text`, `content`, and `messages` MUST be mutually exclusive. A prompt that sets more than one, or that sets none of them, MUST be rejected at config load.

### SD Prompts

For `type: sd`, each prompt MUST use `text`.

- `text` MUST become the SD task `prompt`
- optional `negative_prompt` MUST become the SD task `negative_prompt`
- `content` MUST be rejected at config load
- `messages` MUST be rejected at config load

SD generation parameters other than prompt text, `negative_prompt`, and `steps` (scheduler, cfg, seed, safety checker) MUST follow the existing model-specific heartbeat defaults. SD heartbeat task args MUST set `task_config.image_width` and `task_config.image_height` to `512`.

SD heartbeat entries used for Relay execution-time fitting MUST follow [heartbeat_sd_calibration.md](./heartbeat_sd_calibration.md).

### LLM Prompts

For `type: llm`:

- When `text` is set, Bridge MUST create a single user message whose `content` is that string.
- When `content` is set, Bridge MUST create a single user message whose `content` is the content block list.
- When `messages` is set, Bridge MUST use that message list as the task `messages`.
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

### LLM Messages

When `messages` is set, each message MUST use one of these roles:

- `system`
- `user`
- `assistant`
- `tool`

For `system` and `user`:

- `content` MUST be a non-empty string or a non-empty content block list
- `tool_call_id` MUST NOT be set
- `tool_calls` MUST NOT be set

For `assistant`:

- When `tool_calls` is absent or empty, `content` MUST be a non-empty string or a non-empty content block list
- When `tool_calls` is non-empty, each tool call MUST define `id`, `type` equal to `function`, `function.name`, and `function.arguments`
- `function.arguments` MUST be a JSON object encoded as a string
- `tool_call_id` MUST NOT be set

For `tool`:

- `tool_call_id` MUST be non-empty
- `content` MUST be a non-empty string
- `tool_calls` MUST NOT be set

## Task Args Mapping

### SD

Selected SD prompt fields MUST map to Relay SD inference task args:

- `version` from the task entry `task_version`
- `prompt` from selected prompt `text` or built-in default
- `negative_prompt` from selected prompt `negative_prompt` or built-in default
- `base_model.name` from task `model`
- `task_config.steps` from the selected task entry `steps`
- `task_config.image_width` and `task_config.image_height` `512`
- `task_config.num_images_per_prompt` `1`
- `dtype` from the task entry `dtype` when set; when `dtype` is omitted and either `quantization_config` or `components` is non-empty, Bridge MUST set `dtype` to `bfloat16`; when `dtype`, `quantization_config`, and `components` are all omitted, Bridge MUST omit `dtype`
- `quantization_config` from the task entry `quantization_config` when non-empty, preserving the JSON number literals from the tasks file; otherwise Bridge MUST omit `quantization_config`
- `components` from the task entry `components` when non-empty, preserving the JSON number literals from the tasks file; otherwise Bridge MUST omit `components`

SD heartbeat entries used for Relay execution-time fitting MUST follow [heartbeat_sd_calibration.md](./heartbeat_sd_calibration.md).

### LLM

Selected LLM prompt fields MUST map to Relay GPT inference task args:

- `version` from the task entry `task_version`
- `model` from task `model`
- when the selected prompt uses `text` or `content`, `messages` as a single user message whose `content` is that string or content block list
- when the selected prompt uses `messages`, `messages` as that configured list after tool-history adaptation
- `tools` from the task entry `tools` when the array is non-empty, preserving the JSON number literals from the tasks file; otherwise `tools` MUST be omitted
- `tool_choice` from the task entry `tool_choice` when set, preserving the JSON number literals from the tasks file; otherwise `tool_choice` MUST be omitted
- `response_format` from the task entry `response_format` when set, preserving the JSON number literals from the tasks file; otherwise `response_format` MUST be omitted
- `quantization_config` from the task entry `quantization_config` when non-empty, preserving the JSON number literals from the tasks file; otherwise Bridge MUST omit `quantization_config`

When assistant tool-call history is present, Bridge MUST apply the heartbeat tool-history `function.arguments` adaptation before serializing task args. That adaptation is defined in `docs/model-compatibility/architecture.md` and MUST be implemented by `BuildTemplateToolCallMessages` in the Bridge `llmtask` package. Heartbeat MUST NOT redefine the argument-object versus argument-string rules.

Bridge MUST forward configured `tool_choice` and `response_format` unchanged into Relay GPT task args. Bridge MUST NOT interpret these fields as model behavior.

LLM heartbeat generation config MUST use:

- `max_new_tokens` from the selected task entry `max_new_tokens`
- `do_sample: false`
- `temperature: 0`
- `repetition_penalty: 1.1`
- `dtype: bfloat16`
- a random `seed`
- `template_args.enable_thinking: false`

LLM heartbeat entries used for Relay execution-time fitting MUST follow [heartbeat_llm_calibration.md](./heartbeat_llm_calibration.md).
