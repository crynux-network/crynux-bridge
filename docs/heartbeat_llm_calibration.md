# Heartbeat LLM Tasks for Execution-Time Fitting

This document specifies how Bridge LLM heartbeat entries MUST be composed so Relay can fit the LLM execution-time formula. Task creation, prompt mapping, and generation-config fields MUST follow [heartbeat_tasks.md](./heartbeat_tasks.md). Relay fitting, timeout, and coefficient storage MUST follow `crynux-relay/docs/task_execution_parameters.md`.

This document applies to LLM heartbeat entries. SD heartbeat entries MUST follow [heartbeat_sd_calibration.md](./heartbeat_sd_calibration.md) and MUST NOT use the rules below.

## Formula Inputs Heartbeat Controls

Relay fits:

```
actual_execution_seconds =
    constant_seconds
    + seconds_per_input_byte * text_input_bytes
    + seconds_per_output_token * actual_completion_tokens
    + model_switch_seconds * model_switched
    + seconds_per_image * image_count
    + seconds_per_megapixel * (image_pixels / 1000000)
```

Heartbeat MUST control the workload fields that Bridge stores at task creation and that this set varies: `LLMTextInputBytes` and `LLMMaxNewTokens`. Relay timeout MUST use stored `LLMMaxNewTokens`. Relay fitting MUST use verified `usage.completion_tokens`.

Heartbeat MUST NOT set `model_switched`. Relay records that flag at dispatch.

LLM heartbeat `dtype` MUST remain `bfloat16` as required by [heartbeat_tasks.md](./heartbeat_tasks.md). Heartbeat MUST NOT set `quantize_bits`.

## Independent Workload Axes

Each LLM model that heartbeat emits MUST have its own grid. Entries in that grid MUST share `type` `llm`, `model`, `min_vram`, and `task_version`. `min_vram` MUST be the VRAM required to run that model. Entries MUST differ only by input class and `max_new_tokens`.

Input class MUST be one of:

| Input class | Prompt form | Size | Construction |
|---|---|---|---|
| one-line | `text` | the stop-early or fill-cap instruction, counting each `{{hb_rand}}` as 16 characters | generation instruction only |
| paragraph | `text` | 400 to 800 characters | one paragraph of request context, then a newline, then the generation instruction |
| medium-text | `text` | 3000 characters | a realistic multi-paragraph user request, then a newline, then the generation instruction |
| long-text | `text` | 9000 characters | a realistic long user request, then a newline, then the generation instruction |
| tool-call | `messages` and `tools` | tool result body 8000 to 9500 characters | system message, one user message, one assistant message with `tool_calls`, matching `tool` messages whose `content` is a realistic tool result of the required size, last user message equal to the generation instruction |
| chat-history | `messages` | combined user and assistant text 8000 to 9500 characters | system message, then eight realistic user/assistant turns, last user message equal to the generation instruction |

Sized `text` bodies, tool result bodies, and chat-history user/assistant text MUST read as a single technical request, documentation page, or operator transcript. They MUST NOT be a short phrase repeated to reach the required length. `paragraph` context MUST be ordinary prose.

The sized field MUST reach the required length, including the trailing generation instruction. Length MUST count each `{{hb_rand}}` as 16 characters, which is the hex value Bridge substitutes at task creation. Runtime substitution MUST change only those 16 characters and MUST NOT change prompt length.

Each generation instruction MUST contain `{{hb_rand}}` as a ticket number or other request value inside the instruction sentence. The instruction MUST NOT append a labeled side field such as `Request id:`.

`one-line`, `paragraph`, `medium-text`, and `long-text` entries MUST use prompt `text` and MUST NOT set `content`, `messages`, or `tools`.

`tool-call` entries MUST set `tools` to a single `search_docs` function-tool array. Assistant `tool_calls` and `tool` messages MUST follow [heartbeat_tasks.md](./heartbeat_tasks.md). Two prompts of a `tool-call` entry MUST share the same tool history and MUST differ only by the last user message. The last user message MUST name ticket `{{hb_rand}}` and MUST require a `search_docs` call whose `query` and `limit` depend on the last hex digit of that ticket. The two prompts MUST NOT share one fixed function name plus one fixed arguments object.

`chat-history` entries MUST use prompt `messages` and MUST NOT set `content` or `tools`. Assistant messages MUST have non-empty `content` and MUST NOT set `tool_calls`. Two prompts of a `chat-history` entry MUST share the same prior turns and MUST differ only by the last user message.

Each entry MUST contain exactly two prompts that share the same input class and differ only by generation instruction:

| Output class | Generation instruction | Actual completion tokens |
|---|---|---|
| stop-early | `Ticket {{hb_rand}} is a random hex id. In one short sentence, give an integer score from 0 to 100 for how much you like that id, and name the two hex digits that most influenced the score.` | far below `max_new_tokens` |
| fill-cap | `Ticket {{hb_rand}} is a random hex id. Write continuously until the maximum output length is reached. The first character of your output must be the last hex character of that ticket. Keep inventing new concrete plot details that depend on different pairs of hex digits from the ticket; do not follow a fixed outline.` | equal to `max_new_tokens` |

For `tool-call`, the stop-early last user message MUST be:

```
Ticket {{hb_rand}} needs a follow-up search. If the last hex digit of that ticket is 0, 2, 4, 6, or 8, call search_docs with query equal to the ticket and limit 1. Otherwise call search_docs with query execution timeout {{hb_rand}} and limit 3. Then reply in one sentence that repeats the ticket.
```

The fill-cap last user message MUST be:

```
Ticket {{hb_rand}} needs a follow-up search. If the last hex digit of that ticket is 0, 2, 4, 6, or 8, call search_docs with query equal to the ticket and limit 1. Otherwise call search_docs with query execution timeout {{hb_rand}} and limit 3. Then write the full investigation for that ticket, start every paragraph with the ticket number, and do not stop until the maximum output length is reached.
```

The `tool-call` system message MUST be:

```
You are an ops assistant for a GPU inference network. When the latest user message names a ticket, call search_docs before you answer, and put that ticket into the query.
```

Relay fitting uses verified `usage.completion_tokens`, not `max_new_tokens`. Those two prompts MUST give that entry two different actual completion-token values at the same input bytes and the same `max_new_tokens`. Bridge prompt selection MUST keep both prompts; uniform sampling of the prompt list MUST expose both values.

Each model MUST use exactly three `max_new_tokens` values:

| Token class | `max_new_tokens` |
|---|---|
| `T_low` | greater than 0 and less than `T_mid` |
| `T_mid` | greater than `T_low` and less than `8000` |
| `T_high` | `8000` |

Fill-cap completions at `T_low`, `T_mid`, and `T_high` MUST produce three different actual completion-token counts.

Each model grid MUST contain every pairing of the six input classes with `T_low`, `T_mid`, and `T_high`.

Entries MUST have `LLMImageCount` 0 and `LLMImagePixels` 0.

## Model Switch Samples

Heartbeat MUST emit at least two LLM models whose `min_vram` values allow the same GPU class to run both models. Relay then records `model_switched` when a node loads a different LLM model between tasks.

## Per-model Grid

When heartbeat adds an LLM model, it MUST emit the following eighteen entries. Every entry MUST include both output-class prompts.

| max_new_tokens | input class | prompts |
|---|---|---|
| `T_low` | one-line | stop-early and fill-cap |
| `T_low` | paragraph | stop-early and fill-cap |
| `T_low` | medium-text | stop-early and fill-cap |
| `T_low` | long-text | stop-early and fill-cap |
| `T_low` | tool-call | stop-early and fill-cap |
| `T_low` | chat-history | stop-early and fill-cap |
| `T_mid` | one-line | stop-early and fill-cap |
| `T_mid` | paragraph | stop-early and fill-cap |
| `T_mid` | medium-text | stop-early and fill-cap |
| `T_mid` | long-text | stop-early and fill-cap |
| `T_mid` | tool-call | stop-early and fill-cap |
| `T_mid` | chat-history | stop-early and fill-cap |
| `T_high` | one-line | stop-early and fill-cap |
| `T_high` | paragraph | stop-early and fill-cap |
| `T_high` | medium-text | stop-early and fill-cap |
| `T_high` | long-text | stop-early and fill-cap |
| `T_high` | tool-call | stop-early and fill-cap |
| `T_high` | chat-history | stop-early and fill-cap |
