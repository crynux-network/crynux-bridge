## Document Index

- `heartbeat_tasks.md`: Heartbeat background task creation, backpressure limits, weighted task selection, configurable multi-prompt sampling, LLM text/image content mapping, and SD prompt mapping.
- `heartbeat_llm_calibration.md`: LLM heartbeat workload grid that Relay execution-time fitting requires, including input classes, output classes, and the per-model entry set.
- `heartbeat_sd_calibration.md`: SD heartbeat workload grid that Relay execution-time fitting requires, including the two `steps` classes per model.
- `improve_task_success_rate_with_repeat_tasks.md`: Bridge config-controlled repeat fan-out for logical-task reliability. This is not protocol task validation.
- `task_lifecycle.md`: HTTP request lifetime, Relay-owned deadlines, per-inference-task reconciliation, terminal synchronization, and validation-group stopping rules.
- `task_processing_concurrency.md`: Single-loop lifecycle-write ownership, bounded batch request concurrency, persisted scheduling, restart recovery, and capacity verification.
- `task_engine.md`: Authoritative Bridge TaskEngine design covering the database-driven main loop, bounded I/O workers, task creation and reads, VSS groups, repeat expansion, recovery, and integration boundaries for raw and heartbeat callers. Repeat expansion and VSS expansion are separate paths.
- `sdft_tasks.md`: Existing SDFT multi-round behavior, its boundary from the TaskEngine replacement, and the separate component contract that keeps SDFT branches out of TaskEngine.
- `model-compatibility/`: Bridge raw-task transport boundaries, canonical `GPTTaskArgs` and raw result preservation, heartbeat tool-history adaptation, and the boundaries with Crynux AS and gpt-task.

## Repeat Fan-out and Protocol Validation

`task.repeat_num` controls Bridge repeat expansion: one logical `ClientTask` MUST expand into that many independent primary tasks with distinct `TaskID` values. Those primaries run concurrently, and the first successfully downloaded result completes the logical task. The value MUST come from Bridge configuration, except that Bridge-internal callers such as heartbeat and SDFT MAY override it to `1` on the TaskEngine submission. This mechanism improves success rate, reliability, and effective completion speed. It MUST NOT be treated as task validation: it MUST NOT replace, trigger, or configure VSS or single-task validation, and it MUST NOT decide validation membership or participate in score comparison.

Protocol validation is separate. When VRF selects multi-node validation for one primary, TaskEngine MUST create additional members under that same `TaskID` for result comparison. Whether validation runs and how many members it uses MUST follow the protocol and VRF result, not `repeat_num`. Each repeat primary MAY still enter its own validation path; that path is protocol validation, not the meaning of `repeat_num`.

Details are specified in `improve_task_success_rate_with_repeat_tasks.md` and `task_engine.md`.

## Model Compatibility Authority

Crynux AS public OpenAI-compatible request adaptation, canonical `GPTTaskArgs` creation, and public response normalization MUST use `crynux-as/docs/model-compatibility/` in the standalone AS repository as their authority. Bridge raw-task transport boundaries and heartbeat tool-history adaptation MUST use `docs/model-compatibility/` in this repository.

Prompt rendering, chat templates, processor behavior, rendering of Bridge-adapted heartbeat tool history into model input, thinking template controls, AutoClass, remote `auto_map`, execution backends, tensor-parallel fallback, generation, and raw decoding MUST use `gpt-task/docs/model-compatibility/` in the standalone gpt-task repository as their authority.

Bridge documentation MUST own raw task API behavior, task creation, transport boundaries, raw result delivery, and lifecycle protocols. It MUST NOT redefine gpt-task execution facts. Model pages are verified coverage samples and MUST NOT be implemented as Bridge runtime model-ID allowlists.

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
