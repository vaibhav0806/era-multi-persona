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
