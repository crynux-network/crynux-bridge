## Document Index

- `heartbeat_tasks.md`: Heartbeat background task creation, backpressure limits, weighted task selection, configurable multi-prompt sampling, LLM text/image content mapping, and SD prompt mapping.
- `improve_task_success_rate_with_repeat_tasks.md`: Repeat-task design for improving API task success rate.
- `model-compatibility/`: End-to-end OpenAI-compatible LLM request adaptation, transparent task transport, gpt-task execution boundary, and response normalization.

## Model Compatibility Authority

OpenAI-compatible request adaptation, canonical `GPTTaskArgs` creation, task transport boundaries, and public response normalization MUST use `docs/model-compatibility/` in this repository as their authority.

Prompt rendering, chat templates, processor behavior, rendering of Bridge-adapted tool history into model input, thinking template controls, AutoClass, remote `auto_map`, execution backends, tensor-parallel fallback, generation, and raw decoding MUST use `gpt-task/docs/model-compatibility/` in the standalone gpt-task repository as their authority.

Bridge documentation MUST own public API normalization, task creation, transport boundaries, response normalization, and lifecycle protocols. It MUST NOT redefine gpt-task execution facts. Model pages are verified coverage samples and MUST NOT be implemented as Bridge runtime model-ID allowlists.

## Doc Update Requirements

When updating documentation files:

1. Read the entire document first to understand its structure, sections, and flow
2. Find the most appropriate location to integrate new content based on:
   - Logical relationship with existing sections
   - Document flow and narrative
   - Where readers would naturally expect to find the information
3. Integrate new content naturally into existing sections when possible:
   - Add as a paragraph within a relevant section
   - Extend an existing list or table
   - Add as a subsection under an appropriate parent section
   - Distribute across multiple sections if a feature affects different parts of the document
4. Do NOT simply create a new top-level section and place all new content there
5. Only create a new section if the topic is truly distinct from all existing content

Write documentation as a specification.

Documentation MUST state clear, final decisions and requirements.

Documentation MUST NOT include:
- Recommendations or advice.
- Options or alternatives.
- Speculation or uncertainty.
- Future-facing placeholders.

Documentation MUST use definitive language that can be implemented and tested:
- Requirement keywords: MUST, MUST NOT, SHALL, SHOULD. Use SHOULD only when a requirement level is intended.
- Exact behavior, constraints, and interfaces.

## Chat Content Isolation

Documentation MUST be generated from task requirements and authoritative project sources only.
User chat instructions about removing content are editing actions, not document content.
The final document MUST NOT restate removal instructions.
If a content type is removed, it must be absent from the final document.

Example chat cycle:
- AI draft includes setup commands.
- User says remove setup commands and keep only flow.
- Wrong final doc line: This document does not include setup commands.
- Right final doc line: Run the flow in order: prepare environment, start services, execute deposit and withdraw, then verify results.
