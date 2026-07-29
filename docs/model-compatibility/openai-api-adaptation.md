# OpenAI-Compatible LLM Adaptation

## Endpoint Boundaries

`/v1/llm/chat/completions`, its VRAM-limit variant, and `/v1/openrouter/chat/completions` MUST use the Chat Completions adaptation in this document.

`/v1/llm/completions` MUST create one user message from `prompt`. It does not expose chat roles, tools, tool history, or structured tool-call output.

Both endpoint families MUST complete the network task before constructing their HTTP response. A request with `stream=true` MUST stream the completed normalized response; it is not token-live model streaming.

## Chat Request Mapping

Bridge MUST preserve the order of input messages. The supported roles are `system`, `user`, `assistant`, and `tool`. `developer` is not a canonical `GPTTaskArgs` role and MUST be rejected rather than silently treated as a supported model role.

String content MUST remain a string. Multipart content MUST accept:

- `{"type":"text","text":"..."}` with non-empty text;
- `{"type":"image_url","image_url":{"url":"data:<media-type>;base64,<payload>"}}`.

Bridge MUST convert an accepted `image_url` part to `{"type":"image","base64":"<payload>"}`. The task payload MUST contain valid raw base64 without the media type or data URL prefix. Empty content arrays, unsupported content types, remote image URLs, malformed data URLs, and invalid base64 MUST be rejected.

Bridge MUST map supported request generation fields into `generation_config`. `max_completion_tokens` MUST take precedence over `max_tokens`; the configured Bridge default MUST apply when neither field is present. `n`, `top_p`, `top_k`, `min_p`, `repetition_penalty`, and `stop` MUST retain their documented meanings.

Bridge MUST pass `model`, `tools`, `seed`, dtype, and quantization controls through the canonical task fields. Requests whose model starts with `Qwen/Qwen2.5` MUST use `dtype="bfloat16"`; other Chat Completions and Completions requests MUST use `dtype="auto"` unless another public API rule explicitly sets it.

`tool_choice` is not represented by canonical `GPTTaskArgs`. Bridge MUST NOT claim that this request field controls model behavior.

## Tool-History Mapping

Assistant `tool_calls`, tool `tool_call_id`, message content, and tool-result messages MUST preserve conversation order.

OpenAI-compatible assistant history represents `function.arguments` as a JSON string. Transformers requires function arguments passed to chat templates to be dictionaries. Bridge MUST therefore parse every assistant-history argument string as a JSON object before task serialization by default.

The official DeepSeek V3-family templates, the full DeepSeek R1 template, and the DeepSeek V3.2 encoder require JSON strings. Requests for those interfaces MUST retain the validated argument string. DeepSeek R1 Distill checkpoints MUST use the Transformers object default because they use their base-model templates rather than the full R1 template.

Bridge MUST validate every argument string as a JSON object, including strings retained for a model adapter. Invalid JSON, JSON `null`, arrays, and scalar values MUST reject the request. Adaptation MUST run whenever assistant tool-call history exists and MUST NOT depend on whether the current request includes `tools`.

Template execution does not expose machine-readable argument-type metadata. Runtime trial rendering MUST NOT select the representation because a template can accept a string while silently serializing it as a JSON string instead of an argument object. The documented DeepSeek exception MUST be selected before task serialization. The adapter MUST NOT change function names, call IDs, call order, argument keys, or argument values.

## Template Arguments and Thinking Input

The presence of tools MUST NOT automatically disable model thinking. Bridge MUST NOT add `template_args.enable_thinking=false` solely because the requested model can emit a supported tool-call format.

`template_args` is a canonical task extension map, not a raw field in the current OpenAI-compatible Chat Completions request. Bridge MUST populate it only from controls explicitly defined by a public request mapping. Arbitrary request properties MUST NOT be copied into `template_args`.

Bridge MUST preserve mapped `template_args` when constructing the task payload. Relay and Node MUST transport them unchanged. gpt-task owns whether the selected tokenizer, processor, or prompt adapter accepts, normalizes, or ignores each value.

## Raw Output Normalization

Bridge MUST normalize each raw assistant choice independently.

The normalizer MUST pass the complete raw text to every registered tool-call format adapter. It MUST NOT select an adapter by model ID, remove a thinking prefix before parsing, or require a closing thinking tag before recognizing a tool call.

The initial adapter set MUST support:

1. Hermes JSON inside `<tool_call>...</tool_call>`, with non-empty `name` and `arguments`.
2. Qwen XML inside `<tool_call>...</tool_call>`, with a non-empty `<function=name>` and one or more `<parameter=name>value</parameter>` elements.

Hermes `arguments` MAY be a JSON value or a JSON-encoded string. Bridge MUST return the OpenAI function-arguments string.

Each Qwen parameter value MUST be JSON-decoded when valid and retained as a string otherwise. Bridge MUST serialize the resulting parameter object as the OpenAI function-arguments string.

Adapters MUST recognize all valid ordered call blocks in the raw output, including a call emitted before a closing thinking tag. Multiple successful calls MUST produce multiple OpenAI tool calls in source order.

An adapter that does not recognize a block MUST return no match without mutating the text. Malformed, incomplete, or unsupported blocks MUST remain ordinary content and MUST NOT produce `finish_reason="tool_calls"`.

## Content and Thinking Preservation

For every successfully parsed tool call, Bridge MUST remove only the source tool-call block represented by that structured call. All remaining raw text MUST retain its original order.

`message.content` MUST therefore preserve:

- `<think>` or `<thinking>` content and delimiters;
- final assistant text;
- text surrounding tool calls;
- malformed or unsupported tool-call syntax.

This API contract MUST NOT introduce `reasoning_content` or `reasoning`. Applications receive generated thinking in `message.content` and MAY interpret it according to their own contract.

When at least one call is parsed, Bridge MUST populate `message.tool_calls`, assign stable unique call IDs within the response, and set `finish_reason="tool_calls"`. The existence of structured calls MUST NOT force the remaining `message.content` to an empty string.

## Streaming Response

Chat Completions streaming MUST use the same normalized choices as non-streaming output.

Bridge MUST emit:

1. an initial assistant-role chunk;
2. ordered chunks for the preserved `message.content`;
3. structured tool-call deltas in normalized call order;
4. a terminal choice chunk with the final finish reason;
5. an empty-choice usage chunk when `stream_options.include_usage=true`;
6. `data: [DONE]`.

The streaming path MUST NOT reparse content, strip thinking, or select a different format adapter.

## Format Adapter Extension Contract

A new tool-call format MUST be added as an independent parser adapter. Each adapter MUST:

- identify syntax from the supplied raw block or text;
- validate every field required for an OpenAI function call;
- return the exact matched source range and ordered structured calls on success;
- return no match without mutation on failure;
- avoid model-ID checks;
- include evidence for ordinary content, thinking content, malformed syntax, single calls, and multiple calls.

Adding a format adapter MUST NOT alter canonical `GPTTaskArgs`, gpt-task output, Relay transport, or existing adapter behavior.
