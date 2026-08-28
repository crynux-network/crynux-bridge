# SDFT Tasks

This document records the existing SDFT execution flow, its boundary from the TaskEngine replacement, and the required structure for a separate SDFT implementation.

## Existing Processing Flow

The existing SDFT implementation represents one user request with one `ClientTask` and executes a serial sequence of `InferenceTask` rounds under that record.

The existing `ProcessSDFTTasks` loop:

1. scans running SDFT `ClientTask` records;
2. starts one polling goroutine for each selected client task;
3. waits for the current inference task and its validation group;
4. downloads the successful round output as `checkpoint.zip`;
5. checks the archive for a `FINISH` entry;
6. renames the final checkpoint to `result.zip` and completes the client task when `FINISH` exists;
7. creates another `InferenceTask` under the same `ClientTask` with the downloaded checkpoint when `FINISH` does not exist;
8. increments `ClientTask.FailedCount` after a failed round and creates another round while the count is not greater than `3`.

SDFT differs from a standard TaskEngine logical task because a successfully downloaded inference-task result does not necessarily complete the user request. The checkpoint contents determine whether another serial round is required.

The existing implementation also has SDFT-specific Relay transport:

- the creator supplies the Relay execution timeout;
- an input checkpoint is uploaded with task creation when present;
- the successful Relay artifact is downloaded from the checkpoint endpoint;
- validation and continuation tasks preserve the timeout;
- continuation changes the checkpoint field in the task arguments.

## TaskEngine Replacement Boundary

The TaskEngine replacement MUST NOT implement the existing SDFT client-task processor.

TaskEngine MUST NOT:

- identify an SDFT submission source;
- inspect checkpoint archives;
- check for the `FINISH` entry;
- create serial continuation rounds;
- apply SDFT failure-count rules;
- preserve an SDFT timeout;
- upload an SDFT checkpoint;
- select an SDFT checkpoint download endpoint;
- change client-task completion rules for SDFT.

The existing SDFT APIs, `ProcessSDFTTasks`, SDFT model helpers, Relay transport, checkpoint handling, tests, and startup wiring MUST remain unchanged by the TaskEngine replacement. SDFT behavior MUST NOT be part of the TaskEngine implementation or its acceptance tests.

## SDFT Component Contract

SDFT processing MUST be implemented as a component outside TaskEngine.

The component MUST persist one SDFT job for the complete user request. The SDFT API MUST expose the SDFT job identity rather than using one TaskEngine `ClientTaskID` as the identity of the complete multi-round request.

For each round, the SDFT component MUST:

1. create one independent logical task through the common TaskEngine creation method with `repeat_num` equal to `1`;
2. persist the returned `ClientTaskID` as one round of the SDFT job;
3. read round status through the public TaskEngine status method;
4. read a successful round artifact through the public TaskEngine result method;
5. inspect the checkpoint for the `FINISH` entry;
6. complete the SDFT job when `FINISH` exists;
7. create a new logical task with the downloaded checkpoint when `FINISH` does not exist;
8. count failed rounds and fail the SDFT job after the fourth failed round.

Each continuation round MUST have a distinct TaskEngine `ClientTaskID`. TaskEngine MUST apply its standard rule that the first downloaded result completes that round's `ClientTask`. TaskEngine MUST NOT know whether another SDFT round follows.

The SDFT component MUST own SDFT job state, checkpoint selection, continuation decisions, failed-round counting, API status conversion, and final result selection. It MUST NOT read or write TaskEngine `ClientTask`, `InferenceTask`, operation, VSS, or result metadata records directly.

SDFT creation requires the common task contract and Relay client to support opaque input and output artifacts without checking for `TaskTypeSDFTLora` inside TaskEngine. The SDFT component MUST describe the checkpoint input and expected checkpoint output through that generic contract. TaskEngine MUST process those declarations without SDFT-specific branches.

Process restart MUST recover SDFT jobs from persisted SDFT component state. Recovery MUST read the persisted current-round `ClientTaskID` through TaskEngine before creating another round, so a restart cannot create two continuation rounds for the same checkpoint.
