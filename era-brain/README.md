# era-brain

> Modular agent brain SDK: swappable memory, LLM, iNFT, and identity providers.

`era-brain` is the framework powering [era-multi-persona](../). It defines six interfaces — `Persona`, `Brain`, `MemoryProvider`, `LLMProvider`, `INFTRegistry`, `IdentityResolver` — and ships reference implementations for SQLite, OpenRouter, 0G Storage (KV + Log), 0G Compute (sealed inference), ERC-7857 (forked), and ENS.

See [`examples/coding-agent`](./examples/coding-agent) for a working 3-persona pipeline.

## Install

```bash
go get github.com/vaibhav0806/era-multi-persona/era-brain
```

## Status

M7-A: skeleton + SQLite + OpenRouter impls. iNFT, ENS, and 0G providers are interface-only and arrive in M7-B through M7-E.
