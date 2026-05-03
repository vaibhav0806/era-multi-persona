# era-multi-persona — Demo Video Script

**Target length:** 4:00. **Target audience:** hackathon judges (technical, attention-limited). **Format:** screencast with voiceover. **Delivery:** YouTube unlisted, link goes into all 3 submissions.

## Pre-flight checks (do before recording)

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era

# 1. VPS bot stopped (otherwise it intercepts Telegram messages)
ssh era@178.105.44.3 sudo systemctl stop era
ssh era@178.105.44.3 systemctl is-active era      # expect "inactive"

# 2. Sepolia + 0G Galileo balances
cast balance $(cast wallet address $PI_ZG_PRIVATE_KEY) --rpc-url $PI_ZG_EVM_RPC --ether
cast balance $(cast wallet address $PI_ZG_PRIVATE_KEY) --rpc-url $PI_ENS_RPC --ether

# 3. Build fresh binary
go build -o bin/orchestrator ./cmd/orchestrator

# 4. Pre-position browser tabs:
#    - Telegram web (logged in to your test account)
#    - https://chainscan-galileo.0g.ai/address/0x33847c5500C2443E2f3BBf547d9b069B334c3D16
#    - https://sepolia.app.ens.domains/vaibhav-era.eth
#    - GitHub repo: https://github.com/vaibhav0806/pi-agent-sandbox/pulls
```

Pick a fresh persona name not yet minted. Suggestions: `gopher`, `sqlsmith`, `hackernews`. The script below uses `gopher`.

---

## Beat 1 — Problem + value prop (0:00–0:30)

**Visual:** Title card with logo or repo name. Or just the README's hero section in a browser.

**Voiceover (~60 words):**

> "era-multi-persona is a coding agent that runs on a multi-persona swarm. Every task you send via Telegram fans out to a planner, a coder, and a reviewer — three sealed-inference LLM personas, each with their own evolving memory on 0G Storage, each minted as an iNFT, each addressable via an ENS subname. New personas can be minted live during a task. Let me show you."

---

## Beat 2 — Architecture in 30 seconds (0:30–1:00)

**Visual:** Scroll the README's architecture diagram in the browser. Focus on the three layers (era-brain SDK, era orchestrator, on-chain).

**Voiceover (~50 words):**

> "Three layers: era-brain is a Go SDK exposing five interfaces — Persona, LLMProvider, MemoryProvider, INFTRegistry, IdentityResolver. The orchestrator imports the SDK and wires it into Telegram. On-chain, an ERC-7857 fork on 0G Galileo handles minting and invocation events; ENS subnames on Sepolia handle resolution. All three layers are independent. The SDK is reusable."

---

## Beat 3 — Live `/task` with a default persona (1:00–2:15)

**Visual:** Terminal running `./bin/orchestrator`. Telegram on the side.

**Action (steps in voiceover order):**

1. Show orchestrator boot lines, **highlight these specifically**:
   - `INFO 0G storage wired`
   - `INFO 0G Compute sealed inference wired model=qwen/qwen-2.5-7b-instruct`
   - `INFO 0G iNFT registry wired contract=0x33847c5500C2443E2f3BBf547d9b069B334c3D16`
   - `INFO ENS resolver wired parent=vaibhav-era.eth`

2. In Telegram: `/task add a /healthz endpoint that returns 200 OK`

3. Watch the orchestrator stdout. Point out (don't read each one):
   - "0G Storage append" lines × 4 (planner audit + KV + reviewer audit + KV)
   - "Set tx params" / "Transaction receipt" × 3 (planner, coder, reviewer iNFT recordInvocation)

4. Telegram DM arrives with PR + planner plan + reviewer decision + **`personas:` footer** showing 3 ENS subnames + token IDs.

5. Click the PR URL — show the GitHub PR opened by `era-orchestrator[bot]`.

**Voiceover (~120 words, conversational):**

> "I run the orchestrator. Boot lines confirm 0G Storage, 0G Compute sealed inference, the iNFT registry, and the ENS resolver are all wired. Now I send a /task in Telegram. Watch the orchestrator. There's the planner LLM call — sealed inference, receipt written to 0G Storage, tx submitted to record the Invocation event on the iNFT contract. Coder is Pi-in-Docker doing the actual file edits. Reviewer runs after, same sealed-inference treatment. Telegram DM lands with the PR URL, the planner's plan, the reviewer's verdict, and a personas footer showing the three default personas — planner, coder, reviewer — each resolved live from Sepolia ENS at DM-render time. Click the PR — opened by the bot, against the sandbox repo."

---

## Beat 4 — Mint a custom persona LIVE (2:15–3:15)

**Visual:** Telegram + 0G chainscan + ENS app.

**Action:**

1. Telegram: `/persona-mint gopher You only write idiomatic Go. Use small interfaces, errors as values, no panics in library code. Format with gofmt. Comments answer 'why', not 'what'.`

2. Wait ~30 seconds while orchestrator: uploads prompt to 0G KV → mints iNFT → registers ENS subname → writes 4 text records → inserts SQLite row.

3. Telegram bot replies with: token #N, chainscan link, ENS link, 0G storage URI.

4. **Click the chainscan link.** Show the mint transaction. Highlight: `Transfer(from=0x0, to=signer, tokenId=N)`.

5. **Click the ENS link.** Show the new subname `gopher.vaibhav-era.eth` with 4 text records. Read out the `inft_addr`, `inft_token_id`.

6. Telegram: `/personas` — show the listing now includes `gopher`.

**Voiceover (~110 words):**

> "Now the demo line. I'm minting a new persona live. /persona-mint gopher, with a system prompt that says only idiomatic Go. The orchestrator uploads the prompt blob to 0G storage, mints a new iNFT token on 0G Galileo, registers the ENS subname on Sepolia, writes four text records pointing the subname at the token, and inserts a row in local SQLite. About 30 seconds for all the chain interactions. Bot replies with the chainscan link — there's the mint transaction. ENS app — there's the new subname with all four text records, all on-chain, all verifiable. /personas command shows it in the listing. Now I can use it."

---

## Beat 5 — Use the custom persona + verify (3:15–4:00)

**Visual:** Telegram + chainscan iNFT events page.

**Action:**

1. Telegram: `/task --persona=gopher write a /version endpoint that returns the commit SHA and uptime in seconds`

2. Watch the orchestrator. Note the prompt prefix: `PERSONA SYSTEM PROMPT: You only write idiomatic Go...`

3. Task completes. Telegram DM lands. **Personas footer now shows:**
   - `planner.vaibhav-era.eth → token #0`
   - `gopher.vaibhav-era.eth → token #N`  *(NOT `coder.…`)*
   - `reviewer.vaibhav-era.eth → token #2`

4. Click PR URL. Show the actual code — it should look more idiomatic Go than the previous task (small interfaces, no panics, etc).

5. Open chainscan iNFT contract events page. Filter by tokenID = N. Show new `Invocation` events for the just-completed task.

**Voiceover (~110 words, closing):**

> "Now I use the new persona. /task with the --persona=gopher flag. The orchestrator looks up gopher in the registry, fetches the prompt from local cache, prepends it to Pi's task description. Reviewer flow runs as usual. Telegram DM lands — and notice the personas footer. The middle slot is now gopher.vaibhav-era.eth, token N — not the default coder. I open the PR, the code is idiomatic Go. Back to chainscan — filter by gopher's token ID, and there are the Invocation events from this exact task. Three on-chain prize tracks in one feature: 0G iNFT, 0G framework SDK, ENS integration. Repo, full milestone history, and verify-on-chain links are in the description. Thanks."

---

## Recording tips

- **Mic test.** Voiceovers in noisy rooms ruin hackathon demos. Test 30 sec before recording 4 min.
- **OBS or QuickTime.** OBS for picture-in-picture (you talking + screen). QuickTime for screen-only.
- **Practice once.** Rehearsal cuts retake count from 5+ to 1-2.
- **Don't read the script word-for-word.** It's a guide. Speak naturally.
- **Cuts allowed.** If a chain transaction takes 30 seconds, cut to "30 seconds later" and skip the wait.
- **Captions for chain links.** Add overlays with the contract address + ENS name when you click them.

## Upload checklist

1. Render at 1080p, mp4, 4-5 minutes.
2. Upload to YouTube as **Unlisted** (not Private — judges need to view without sign-in; not Public — controls discoverability).
3. Description should include:
   - Repo URL: https://github.com/vaibhav0806/era-multi-persona
   - iNFT contract: https://chainscan-galileo.0g.ai/address/0x33847c5500C2443E2f3BBf547d9b069B334c3D16
   - ENS parent: https://sepolia.app.ens.domains/vaibhav-era.eth
   - All three prize track names

4. Get the share URL — paste into all 3 hackathon submissions.

## Backup plan

If a chain transaction fails during recording (KV node throttle, RPC timeout):
- **For mint**: pre-mint one persona before recording, use that token in Beat 4 (just don't say "minting live" — say "minted just before this recording, here's the proof on chain").
- **For task**: re-run the task. Don't show the failed attempt.

If 0G Compute sealed inference fails: orchestrator falls back to OpenRouter automatically. Receipt shows `Sealed=false` but task still completes. Don't dwell on this in the demo — it works in degraded mode. If you want to highlight the resilience, mention it briefly.
