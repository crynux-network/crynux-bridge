## Coding Requirements

Before adding new code, first check whether existing logic can be reused. Prefer extracting reusable code into a dedicated function, class, or file, and place it in the most appropriate location. Remove duplicated code and avoid adding redundant implementations of the same functionality.

Comments must describe final behavior only. Do not add comments that explain change history, such as what was added, removed, or why code was deleted. Keep comments concise and use them only for complex or non-obvious logic.

## Debug Requirements

Follow a root-cause-first bugfix protocol: analyze the code path, form a clear hypothesis, validate it, and then apply a precise and minimal fix tied directly to the confirmed cause. Avoid speculative or broad defensive changes.

## E2E Test

For E2E test execution instructions, use `tests/e2e/AGENTS.md` as the source of truth.
