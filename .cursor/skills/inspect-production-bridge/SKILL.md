---
name: inspect-production-bridge
description: Inspect the crynux-bridge production server logs and database records over SSH. Use ONLY when the user explicitly asks to check the production bridge server, its logs, or its production database. Never use this skill proactively or for any other purpose.
---

# Inspect Production Bridge Server

Read-only inspection of the crynux-bridge production deployment: application logs and the local MySQL database co-deployed in the same docker-compose stack.

## Usage Restrictions

- This skill MUST NOT be used unless the user explicitly asks to inspect the production bridge server, logs, or database.
- All access is strictly READ-ONLY. You MUST NOT modify, create, delete, move, or overwrite anything on the server: no file writes, no config changes, no container restarts, no `docker compose up/down/restart`, no package installs.
- The database MUST be queried with `SELECT` (and `SHOW`/`DESCRIBE`/`EXPLAIN`) statements only. Never run `INSERT`, `UPDATE`, `DELETE`, `ALTER`, `DROP`, `TRUNCATE`, or any other write statement.
- You MUST stay inside the deployment directory `~/crynux-bridge`. Do not `cd` into or read any other directory on the server (no `/etc`, no other home directories, no other projects). This host also runs unrelated services; do not inspect them.
- Do not print or copy secret files (`app.gpg`, `priv_key.txt`, `decrypt.sh` contents, or anything under `config/`) into the conversation. Do not print `docker-compose.yml` into the conversation.

## Connection

The server is reachable via a preconfigured SSH alias:

```bash
ssh cnx-bridge "<command>"
```

Always run commands non-interactively through `ssh cnx-bridge "..."` and prefix them with `cd ~/crynux-bridge && `. The `ubuntu` user has passwordless `sudo`, which is needed to read log files owned by root and to run `docker compose logs` / `docker compose exec`. Only use `sudo` for those read operations.

## Deployment Layout

Everything lives in `~/crynux-bridge`:

- `docker-compose.yml`: two services:
  - `mysql` (container name `crynux_bridge_mysql`): local MySQL 8.1.0, database `bridge`, not published to the host.
  - `bridge` (container name `crynux_bridge`): application container, port `127.0.0.1:5028`.
- `data/bridge/logs/`: file-based application logs (root-owned, require `sudo` to read).
- `data/bridge/inference_tasks/`: task result artifacts on disk.
- `data/mysql/`: MySQL data volume. Do not browse or modify it.
- `config/config.yml`: bridge configuration. Do not print it.
- `bridge_logs.sh`: helper that follows container logs (`sudo docker compose logs -f -n 100 bridge`). Avoid the `-f` form in automation; use bounded variants below.

## Viewing Logs

Container logs (recent, non-following):

```bash
ssh cnx-bridge "cd ~/crynux-bridge && sudo docker compose logs -n 200 bridge"
```

File logs under `data/bridge/logs/` (root-owned, require `sudo`):

- `ig_server.log`: main application log (logrus format, JST timestamps).
- `ig_server_db.log` and rotated `ig_server-*.log.gz`: GORM SQL logs and rotated app logs; filter with `grep`/`zgrep` instead of reading whole files.
- `crynux_bridge_llm_api_requests.info.log` / `.error.log`: LLM API request logs.
- `crynux_bridge_llm_api_request_toolcalls.info.log` / `.error.log`: tool-call request logs.

```bash
ssh cnx-bridge "cd ~/crynux-bridge && sudo tail -n 200 data/bridge/logs/ig_server.log"
ssh cnx-bridge "cd ~/crynux-bridge && sudo grep 'level=error' data/bridge/logs/ig_server.log | tail -n 50"
ssh cnx-bridge "cd ~/crynux-bridge && sudo zgrep '<pattern>' data/bridge/logs/ig_server-<timestamp>.log.gz | tail -n 50"
```

## Querying the Database

The production database is MySQL running as the `mysql` service in the same docker-compose stack. Database name `bridge`, user `bridge`, password `bridgepass`.

There is no `connect_db.sh`. Database queries MUST be executed by piping a local temporary SQL file into `sudo docker compose exec -T mysql mysql ...` over SSH. Do not inline SQL inside the SSH command.

PowerShell workflow:

```powershell
$sqlFile = New-TemporaryFile
@'
SELECT ...;
'@ | Set-Content -Path $sqlFile -NoNewline
Get-Content -Raw $sqlFile | ssh cnx-bridge "cd ~/crynux-bridge && sudo docker compose exec -T mysql mysql -ubridge -pbridgepass -D bridge"
Remove-Item $sqlFile
```

Examples:

```powershell
$sqlFile = New-TemporaryFile
@'
SHOW TABLES;
'@ | Set-Content -Path $sqlFile -NoNewline
Get-Content -Raw $sqlFile | ssh cnx-bridge "cd ~/crynux-bridge && sudo docker compose exec -T mysql mysql -ubridge -pbridgepass -D bridge"
Remove-Item $sqlFile

$sqlFile = New-TemporaryFile
@'
SELECT id, task_id_commitment, status, task_type, created_at
FROM inference_tasks
ORDER BY id DESC
LIMIT 10;
'@ | Set-Content -Path $sqlFile -NoNewline
Get-Content -Raw $sqlFile | ssh cnx-bridge "cd ~/crynux-bridge && sudo docker compose exec -T mysql mysql -ubridge -pbridgepass -D bridge"
Remove-Item $sqlFile
```

This file-based workflow is required on Windows PowerShell to avoid nested SSH and SQL quoting errors. SQL string literals MUST be written normally inside the SQL file.

Always add `LIMIT` to queries against large tables (`inference_tasks`, `client_tasks`, `old_inference_tasks`, `synced_blocks`).

Known tables: `base_models`, `client_api_keys`, `client_tasks`, `clients`, `inference_tasks`, `lora_models`, `migrations`, `old_inference_tasks`, `synced_blocks`.
