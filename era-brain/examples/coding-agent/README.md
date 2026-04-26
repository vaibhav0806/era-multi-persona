# coding-agent example

Demonstrates a 3-persona flow (planner → coder → reviewer) using era-brain.

## Run

```bash
export OPENROUTER_API_KEY=sk-or-v1-...
go run ./examples/coding-agent --task="add a /healthz endpoint that returns 200 OK"
```

If you're running this from the era-multi-persona repo where the existing `.env` already has `PI_OPENROUTER_API_KEY`, that works too — the example falls back to it.

## What you'll see

- **Planner** lists the steps to implement the task.
- **Coder** produces a unified diff implementing those steps.
- **Reviewer** critiques the diff and ends with `DECISION: approve` or `DECISION: flag`.
- Each persona's receipt prints below its output (model, sealed flag, hash).

## What this is NOT

This example does **not** edit real files or open PRs — that integration lives in the [era orchestrator](../../..) and arrives in M7-A.5. The point of this example is to validate the era-brain abstraction in-process.

## Run with 0G Compute sealed inference

All three personas (planner, coder, reviewer) can be routed through a 0G Compute TEE-attested provider with OpenRouter as fallback. When the provider returns a TEE-signature header (`ZG-Res-Key`), each persona receipt shows `sealed=true`.

### 1. Install 0g-compute-cli

```bash
npm install -g @0glabs/0g-serving-broker
```

### 2. Deposit ZG to broker main account

```bash
0g-compute-cli account deposit --amount 3
```

### 3. Transfer to provider sub-account

```bash
# List available inference services to pick a provider address
0g-compute-cli inference list-services

# Transfer ≥1 ZG to your chosen provider
0g-compute-cli account transfer --provider <PROVIDER_ADDRESS> --amount 1
```

### 4. Generate bearer token

```bash
0g-compute-cli inference get-secret --provider <PROVIDER_ADDRESS>
# Outputs: app-sk-<HEX>  →  this is PI_ZG_COMPUTE_BEARER
```

### 5. Get provider endpoint + model

```bash
0g-compute-cli inference get-service-metadata --provider <PROVIDER_ADDRESS>
# Outputs: endpoint=https://...  model=qwen-2.5-7b-instruct
# endpoint → PI_ZG_COMPUTE_ENDPOINT
# model    → PI_ZG_COMPUTE_MODEL
```

### 6. Set env vars and run

```bash
export PI_ZG_COMPUTE_ENDPOINT=https://your-provider-url
export PI_ZG_COMPUTE_BEARER=app-sk-yourhextoken
export PI_ZG_COMPUTE_MODEL=qwen-2.5-7b-instruct
export OPENROUTER_API_KEY=sk-or-v1-...   # fallback if 0G Compute fails

go run ./examples/coding-agent \
  --task="add a /healthz endpoint that returns 200 OK with body \"ok\"" \
  --zg-compute
```

Expected output: three persona sections (planner, coder, reviewer), each ending with a receipt line showing `sealed=true`. If 0G Compute is unreachable, the fallback fires and a `[zg_compute fell back to openrouter: ...]` line appears on stderr — output still completes via OpenRouter with `sealed=false`.
