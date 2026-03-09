## Build the Docker image for E2E testing

- Goal: Build the local image `crynux-bridge:e2e` from the current repository source code.

- Workflow:
  1. Ensure the current working directory is the repository root.
  2. Build the image from `build/crynux_bridge.Dockerfile` with tag `crynux-bridge:e2e`.
  3. Verify `crynux-bridge:e2e` exists locally.

## Prepare the mount folder for the Docker image

- Goal: Prepare bridge host-side files in a Docker mount workspace for local e2e runs.

- Workspace rule:
  - Choose one mount workspace root directory and use it consistently in Docker volume mappings.
  - Reuse existing files in that workspace when possible instead of recreating everything.

- Required bridge structure:

```text
<mount-root>/
  config/
    config.yml
  privkey.txt
```

- Workflow:
  1. Create the `config` folder under `<mount-root>` if it does not already exist.
  2. Copy `tests/e2e/config.e2e.yml` to `<mount-root>/config/config.yml`.
  3. Mount `<mount-root>/config` to `/app/config` in the bridge container.
  4. Mount `<mount-root>/privkey.txt` to `/app/privkey.txt` in the bridge container.

## Prepare the database

- Goal: Prepare a database instance for the bridge service during e2e runs.

- Database requirements:
  - A database instance must be available for the bridge container.
  - Configure the database connection in `<mount-root>/config/config.yml`.

## Prepare the bridge wallet account

- Goal: Prepare the wallet private key and account information required by the bridge container.

- Required wallet file:

```text
<mount-root>/privkey.txt
```

- Wallet account requirements:
  - `blockchain.account.address` in `<mount-root>/config/config.yml` must be set to the bridge wallet address used for e2e.
  - The private key for that address must be stored in `<mount-root>/privkey.txt` as a single-line hex value.
  - A `0x` prefix is allowed in `<mount-root>/privkey.txt`.
  - The key material in `<mount-root>/privkey.txt` must not contain trailing whitespace.


## Top up the Relay Account for the bridge wallet account

- Goal: Ensure the bridge wallet has enough Relay Account balance before starting e2e.

- Important note:
  - The bridge wallet does not need an application-specific on-chain token balance for bridge e2e.
  - The bridge wallet must have enough balance in its Relay Account for task creation and execution.
  - The Relay Account balance is separate from the wallet's on-chain native token balance.

- Workflow:
  1. Use the bridge wallet address from `<mount-root>/config/config.yml`.
  2. Check the current Relay Account balance for that address from the local Relay service.
  3. If the Relay Account balance is lower than the minimum deposit amount allowed by the relay, read the deposit address and the minimum deposit amount from the Relay configuration.
  4. Send a native token transfer on the target blockchain to the relay deposit address from the same bridge wallet address.
  5. Use at least the minimum deposit amount allowed by the relay for that transfer.
  6. Wait for Relay to observe the successful transfer and credit the transferred amount to the sender's Relay Account.
  7. Re-check the Relay Account balance and confirm it increased before running e2e.
