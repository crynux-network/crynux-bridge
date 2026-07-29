# Model Compatibility Documentation

## Document Index

- `architecture.md`: End-to-end ownership from OpenAI-compatible request normalization through raw model execution to OpenAI-compatible response normalization.
- `openai-api-adaptation.md`: Exact Bridge request mapping, tool-history adaptation, raw-output parsing, thinking preservation, and response construction.

## Authority

This directory is the authoritative specification for OpenAI-compatible LLM request adaptation, task transport boundaries, model execution ownership, and response normalization across Bridge, Relay, Node, Worker, and gpt-task.

Bridge owns OpenAI-compatible request and response adaptation. The standalone gpt-task repository owns the internal execution contract from canonical `GPTTaskArgs` to raw assistant text.

Relay, Node, and Worker MUST preserve canonical task arguments and raw task results. Their component documentation MUST define validation, persistence, scheduling, transport, worker management, GPU selection, and lifecycle behavior without redefining prompt or response semantics.

## Update Requirements

Documents in this directory MUST:

1. Treat the OpenAI-compatible API and canonical `GPTTaskArgs` as separate contracts.
2. Identify every transformation and its owning component.
3. Keep Relay and Node transparent to message, tool, thinking, and generated-output syntax.
4. Keep model-input rendering and inference behavior aligned with `gpt-task/docs/model-compatibility/`.
5. Specify output-format parsing by syntax capability rather than requested model ID.
6. Preserve raw generated text except for syntax blocks successfully converted into structured API fields.

Documentation MUST state final, testable behavior. It MUST NOT contain recommendations, alternatives, speculation, future placeholders, or runtime model allowlists.
