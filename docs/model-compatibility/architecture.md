# Model Compatibility Architecture

## Scope

The Crynux AS raw-task model compatibility flow has three processing stages:

1. Crynux AS converts an OpenAI-compatible request into canonical `GPTTaskArgs`.
2. gpt-task converts `GPTTaskArgs` into model input, executes inference, and returns raw assistant text.
3. Crynux AS converts raw assistant text into an OpenAI-compatible response.

Relay, Node, and Worker carry the contracts between these stages. They MUST NOT add a fourth prompt or output-processing stage.

## Stage 1: OpenAI Request to TaskArgs

For the AS raw-task path, Crynux AS owns the public API contract. Bridge MUST accept the canonical raw-task JSON after schema validation and MUST NOT repeat the public request conversion.

Crynux AS MUST normalize:

- message roles and ordered message history;
- string and multipart content;
- image data URLs into raw-base64 image blocks;
- tool definitions, assistant tool-call history, and tool results;
- template arguments produced by AS-owned public request mappings;
- supported generation parameters;
- seed, dtype, quantization, and AS-owned execution defaults.

An API representation MAY require adaptation before it satisfies a model template contract. Such adaptation remains an AS API concern when it converts an OpenAI field representation without changing the conversation's meaning.

OpenAI assistant tool history represents `function.arguments` as a JSON string, while the Transformers chat-template contract requires a JSON object. Crynux AS MUST parse argument strings into objects by default before task serialization. A documented model interface that requires a string MAY retain the validated OpenAI representation.

This representation adapter belongs to Crynux AS for requests accepted by AS.

Canonical `GPTTaskArgs` supports optional `tool_choice`, `response_format`, and `template_args`. Bridge MUST preserve these fields unchanged in the raw-task path. Bridge MUST NOT interpret them as model behavior; gpt-task owns their compatibility with the selected tokenizer, processor, or family adapter.

Bridge MUST NOT parse a tokenizer chat template, select an AutoClass, construct model-specific prompt text, infer tensor-parallel eligibility, or construct a structured-output constraint.

## Transparent Transport

Bridge MUST serialize canonical `GPTTaskArgs` as the network task payload.

Relay MUST validate, persist, schedule, and deliver that payload without interpreting messages, images, tools, tool history, template arguments, or model output syntax.

Node MUST preserve task arguments while selecting GPUs, configuring the executor, managing Worker processes, and applying Node-owned fallback configuration.

Worker MUST deserialize `GPTTaskArgs`, invoke the configured gpt-task entrypoint, and return its response. Worker MUST NOT normalize OpenAI requests or parse generated tool calls.

Raw gpt-task response choices MUST remain unchanged while Relay, Node, and Bridge task transport stores or transfers them.

## Stage 2: TaskArgs to Raw Text

gpt-task owns model execution. It MUST:

1. Validate canonical `GPTTaskArgs`.
2. Establish deterministic execution and generation configuration.
3. Resolve model, tokenizer or processor, AutoClass, and classic or tensor-parallel execution.
4. Render canonical messages through the selected prompt adapter or multimodal processor.
5. Execute generation.
6. Decode generated tokens into raw assistant text and report usage.

Model-specific chat templates, prompt adapters, image processors, thinking template arguments, remote `auto_map`, tensor-parallel plans, and fallback behavior belong to this stage. Their authoritative specification is `gpt-task/docs/model-compatibility/`.

gpt-task MUST preserve generated thinking and tool-call syntax in its raw assistant text. It MUST NOT create OpenAI `tool_calls` or remove generated reasoning.

## Stage 3: Raw Text to OpenAI Response

Crynux AS owns response normalization for requests submitted through its raw-task path. Bridge owns response normalization only for requests accepted by Bridge's legacy OpenAI-compatible endpoints.

Crynux AS MUST inspect the complete raw assistant text with registered output-format adapters. Adapter selection MUST depend on generated syntax, not the requested model ID. A successful adapter result MUST become an ordered OpenAI tool call.

Only syntax blocks successfully converted into structured fields MAY be removed from `message.content`. Thinking text, final text, and unrecognized or malformed text MUST remain in their original order. Crynux AS MUST NOT discard text solely because it occurs before `</think>` or `</thinking>`.

When one or more tool calls are parsed, Crynux AS MUST set `finish_reason="tool_calls"`. When no tool call is parsed, AS MUST preserve the gpt-task finish reason or apply the endpoint's documented default.

AS streaming is response transport over the completed normalized result. Streaming and non-streaming requests MUST therefore share the same request adaptation, raw-output parsing, content preservation, tool-call order, finish reason, and usage.

## Compatibility Boundaries

Crynux AS compatibility is verified at the public API boundaries:

- OpenAI request to canonical `GPTTaskArgs`;
- raw gpt-task response to OpenAI response.

gpt-task compatibility is verified at the execution boundaries:

- canonical `GPTTaskArgs` to rendered model input;
- loaded model capabilities to execution backend;
- generated token IDs to raw assistant text.

Model sample documentation MUST be evidence for these general contracts. It MUST NOT become a runtime model-ID allowlist.
