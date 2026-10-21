# Model Compatibility Architecture

## Scope

Bridge is a raw-task transport and lifecycle component. The model compatibility flow that crosses Bridge has three processing stages, and Bridge owns none of the prompt or response semantics:

1. The caller produces canonical `GPTTaskArgs`. Crynux AS produces them from its public OpenAI-compatible requests; other raw callers submit them directly.
2. gpt-task converts `GPTTaskArgs` into model input, executes inference, and returns raw assistant text.
3. The caller interprets the raw assistant text. Crynux AS converts it into its public OpenAI-compatible response.

Bridge, Relay, Node, and Worker carry the contracts between these stages. They MUST NOT add a fourth prompt or output-processing stage.

The public OpenAI-compatible API contract, canonical `GPTTaskArgs` creation, and public response normalization are specified by `crynux-as/docs/model-compatibility/`. The execution contract from canonical `GPTTaskArgs` to raw assistant text is specified by `gpt-task/docs/model-compatibility/`.

## Raw-Task Acceptance

Bridge accepts canonical `GPTTaskArgs` through the raw task API (`POST /v1/inference_tasks` and `POST /v1/inference_tasks/batch`) and through the heartbeat component. Bridge MUST accept the canonical raw-task JSON after schema validation and MUST NOT perform any OpenAI request conversion.

Bridge MUST preserve these canonical fields unchanged:

- ordered `messages`, including string and multipart content, image blocks, assistant `tool_calls`, and tool results;
- `tools`;
- `tool_choice`;
- `response_format`;
- `template_args`;
- `generation_config`, `seed`, `dtype`, and quantization controls;
- `model` and `version`.

Bridge MUST NOT interpret `tool_choice`, `response_format`, or `template_args` as model behavior; gpt-task owns their compatibility with the selected tokenizer, processor, or family adapter.

Bridge MUST NOT parse a tokenizer chat template, select an AutoClass, construct model-specific prompt text, infer tensor-parallel eligibility, or construct a structured-output constraint.

## Transparent Transport

Bridge MUST serialize canonical `GPTTaskArgs` as the network task payload.

Relay MUST validate, persist, schedule, and deliver that payload without interpreting messages, images, tools, tool history, template arguments, or model output syntax.

Node MUST preserve task arguments while selecting GPUs, configuring the executor, managing Worker processes, and applying Node-owned fallback configuration.

Worker MUST deserialize `GPTTaskArgs`, invoke the configured gpt-task entrypoint, and return its response. Worker MUST NOT normalize OpenAI requests or parse generated tool calls.

## Raw Result Handling

Raw gpt-task response choices MUST remain unchanged while Relay, Node, and Bridge store or transfer them.

Bridge MUST download the single JSON result of an LLM task and serve it unchanged through `GET /v1/inference_tasks/{client_task_id}/llm`. Bridge MUST NOT parse generated tool calls, strip or relocate generated thinking text, construct OpenAI `tool_calls`, or alter the finish reason.

## Heartbeat Tool-History Adaptation

The heartbeat component creates LLM tasks from configured prompts whose messages MAY contain assistant `tool_calls`. A configured assistant tool call stores `function.arguments` as a JSON object encoded as a string. The Transformers chat-template contract requires a JSON object by default, so Bridge owns this representation adaptation for heartbeat tasks. The adaptation is implemented by `BuildTemplateToolCallMessages` in the Bridge `llmtask` package and MUST run before the task arguments are serialized.

Bridge MUST apply these rules:

- The adaptation MUST run whenever the message list contains an assistant message with `tool_calls`, and MUST NOT depend on whether `tools` is set. A message list without assistant tool-call history MUST be left unchanged.
- By default, Bridge MUST parse every assistant-history `function.arguments` string as a JSON object and serialize it as an object.
- The official DeepSeek V3-family templates, the full DeepSeek R1 template, and the DeepSeek V3.2 encoder require JSON strings. For those model interfaces Bridge MUST retain the validated argument string. DeepSeek R1 Distill checkpoints MUST use the object default because they use their base-model templates.
- Bridge MUST validate every argument string as a JSON object, including strings retained for a model interface. Invalid JSON, JSON `null`, arrays, and scalar values MUST be rejected.
- Runtime trial rendering MUST NOT select the representation, because a template can accept a string while silently serializing it as a JSON string instead of an argument object. The representation MUST be selected before task serialization.
- The adaptation MUST NOT change roles, content, function names, call IDs, call order, argument keys, or argument values.

Heartbeat MUST set `template_args.enable_thinking=false` on every LLM heartbeat TaskArgs. Heartbeat MUST forward configured `tool_choice` and `response_format` unchanged.

Tool-history representation for tasks created by Crynux AS is owned by Crynux AS and is specified by `crynux-as/docs/model-compatibility/`. Bridge MUST NOT apply this adaptation to raw-API submissions.

## Compatibility Boundaries

Bridge compatibility is verified at these boundaries:

- canonical `GPTTaskArgs` accepted by the raw task API to the serialized network task payload, with every canonical field preserved;
- heartbeat prompt messages to canonical `GPTTaskArgs` messages after tool-history adaptation;
- raw gpt-task result stored by Relay to the JSON returned by the raw result endpoint, unchanged.

gpt-task compatibility is verified at the execution boundaries:

- canonical `GPTTaskArgs` to rendered model input;
- loaded model capabilities to execution backend;
- generated token IDs to raw assistant text.

Model sample documentation MUST be evidence for these general contracts. It MUST NOT become a runtime model-ID allowlist.
