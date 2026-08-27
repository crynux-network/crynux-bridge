# Task Processing Concurrency

This document specifies the concurrency behavior of the current Bridge task-processing implementation and the boundary of the Raw Task API update.

## Worker Ownership

`ProcessTasks` MUST use its process-local `sync.Map` to prevent two primary workers in the same Bridge process from being started for the same local `InferenceTask.ID`.

This map does not provide exclusive write ownership for an `InferenceTask` row. A primary worker that processes a validation group calls `syncTaskGroup`, which reads Relay state and writes local state for every member of that group. A member can therefore be written by its own primary worker and by another member's primary worker.

The map is local to one Bridge process. It does not coordinate writers across Bridge processes.

## Inference Task Writes

Relay query responses are applied with ordinary model updates. The update does not compare the stored status with the status that the worker previously read.

The following write order can replace newer local state with older Relay data:

1. One worker queries Relay and receives a task status.
2. Another worker downloads the result and stores `ResultDownloaded`.
3. The first worker applies its earlier Relay response.
4. The stored status becomes the status from the earlier response.

The Relay not-found path also writes a local terminal status. That write can occur while another worker is downloading a result or storing `ResultDownloaded`.

## Client Task Writes

Each member worker can call `updateClientTaskStatus` for the shared `ClientTask`. The function reads the current `ClientTask` and its member tasks, then writes `Status` and, on failure, `FailedCount`.

These reads and writes are not one conditional database operation. Two member workers can read `Running`, calculate updates from different snapshots, and then write the same `ClientTask`. The final `Status` and `FailedCount` depend on the order of those writes.

`ProcessTasks` scans locally unfinished inference tasks. It does not scan all `Running` client tasks to repair their status. If Bridge exits after all member tasks finish but before a member worker updates the client task, that client task can remain `Running` after restart.

## Ownership Boundary

`InferenceTask.Status`, including the local `ResultDownloaded` value, `ClientTask.Status`, and `ClientTask.FailedCount` are Bridge database state. Relay supplies network task state but does not serialize or repair these Bridge writes.

The Raw Task API update MUST retain this processing behavior. It MUST NOT add status compare-and-swap updates, a single database writer for each task, or a client-task recovery scan. Raw status and result reads MUST select from one current database snapshot without writing processing state.
