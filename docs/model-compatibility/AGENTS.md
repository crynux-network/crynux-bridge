# Model Compatibility Documentation

## Document Index

- `architecture.md`: Bridge raw-task transport boundaries: canonical `GPTTaskArgs` preservation, transparent task transport, unchanged raw result handling, heartbeat tool-history adaptation, and the boundaries with Crynux AS and gpt-task.

## Authority

This directory is the authoritative specification for Bridge raw-task transport boundaries and Bridge-owned heartbeat tool-history adaptation.

Crynux AS owns request and response adaptation for its public OpenAI-compatible APIs, and its authority is `crynux-as/docs/model-compatibility/` in the standalone AS repository. Bridge MUST preserve canonical raw-task fields and raw results for AS and every other raw caller. The standalone gpt-task repository owns the internal execution contract from canonical `GPTTaskArgs` to raw assistant text; its authority is `gpt-task/docs/model-compatibility/`.

Relay, Node, and Worker MUST preserve canonical task arguments and raw task results. Their component documentation MUST define validation, persistence, scheduling, transport, worker management, GPU selection, and lifecycle behavior without redefining prompt or response semantics.

## Update Requirements

Documents in this directory MUST:

1. Treat the public OpenAI-compatible API and canonical `GPTTaskArgs` as separate contracts, with the public API owned by Crynux AS.
2. Identify every transformation and its owning component.
3. Keep Relay and Node transparent to message, tool, thinking, and generated-output syntax.
4. Keep model-input rendering and inference behavior aligned with `gpt-task/docs/model-compatibility/`.
5. Keep raw generated text unchanged inside Bridge.

Documentation MUST state final, testable behavior. It MUST NOT contain recommendations, alternatives, speculation, future placeholders, or runtime model allowlists.
