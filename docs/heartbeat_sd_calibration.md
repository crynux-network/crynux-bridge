# Heartbeat SD Tasks for Execution-Time Fitting

This document specifies how Bridge SD heartbeat entries MUST be composed so Relay can fit the SD execution-time formula. Task creation and prompt mapping MUST follow [heartbeat_tasks.md](./heartbeat_tasks.md). Relay fitting, timeout, and coefficient storage MUST follow `crynux-relay/docs/task_execution_parameters.md`.

This document applies to SD heartbeat entries. LLM heartbeat entries MUST follow [heartbeat_llm_calibration.md](./heartbeat_llm_calibration.md) and MUST NOT use the rules below.

## Formula Inputs Heartbeat Controls

Relay fits:

```
actual_execution_seconds =
    overhead_seconds
    + seconds_per_sd_pixel_step * SDUnits
```

`SDUnits` is `num_images * image_width * image_height * steps`. Heartbeat MUST control `steps` on each SD entry. Heartbeat MUST set `num_images` to `1`. Heartbeat MUST omit `image_width` and `image_height` so Relay stores the SD default `512 * 512` pixel area.

Scheduler, `cfg`, seed, safety checker, and model variant MUST remain the model-specific heartbeat defaults specified in [heartbeat_tasks.md](./heartbeat_tasks.md). Those fields MUST NOT be calibration-key fields and MUST NOT differ across entries of the same model.

SD heartbeat MUST NOT set `max_new_tokens` or `tools`.

## Independent Workload Axis

Each SD model that heartbeat emits MUST have its own grid. Entries in that grid MUST share `type` `sd`, `model`, `min_vram`, and `task_version`. `min_vram` MUST be the VRAM required to run that model. `task_version` MUST be the SD inference task version used by heartbeat SD tasks. Entries MUST differ only by `steps`.

Each model MUST use exactly two `steps` values:

| Step class | `steps` |
|---|---|
| `S_low` | `1` |
| `S_high` | greater than `S_low` |

`S_high` MUST be `4` for `crynux-network/sdxl-turbo`. `S_high` MUST be `25` for `crynux-network/stable-diffusion-v1-5`.

Those two `steps` values MUST produce two different `SDUnits` values for the same model, GPU name, GPU VRAM, variant, and dtype, so they update one Relay calibration record.

## Per-model Grid

When heartbeat adds an SD model, it MUST emit the following two entries. Entries of one model MUST share the same prompt list.

| steps | prompts |
|---|---|
| `S_low` | the model's configured prompt list |
| `S_high` | the same prompt list |

Prompt text MUST follow [heartbeat_tasks.md](./heartbeat_tasks.md). Prompt text MUST NOT change `SDUnits`.
