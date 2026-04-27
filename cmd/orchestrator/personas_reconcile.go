package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857"
	"github.com/vaibhav0806/era/internal/queue"
)

// TransferScanner is the orchestrator's view of the iNFT Transfer-event scan.
// *zg_7857.Provider implements this interface implicitly via ScanNewMints.
// Defined locally so tests can stub without spinning up a chain backend.
type TransferScanner interface {
	ScanNewMints(ctx context.Context, sinceTokenID int64) ([]TransferEvent, error)
}

// TransferEvent is re-exported from zg_7857 so the orchestrator-side
// TransferScanner interface and its tests can refer to a single concrete
// type — *zg_7857.Provider satisfies TransferScanner without any adapter.
type TransferEvent = zg_7857.TransferEvent

// personasReconcile runs three idempotent passes at boot to ensure SQLite is
// consistent with on-chain state. Each pass logs failures and continues —
// boot must not crash if any reconcile pass fails.
func personasReconcile(
	ctx context.Context,
	registry queue.PersonaRegistry,
	scanner TransferScanner, // may be nil
	ensWriter queue.ENSWriter, // may be nil
	storage queue.PromptStorage, // may be nil
) {
	if registry == nil {
		slog.Warn("personas: nil registry, skipping reconcile")
		return
	}
	if err := reconcileDefaults(ctx, registry); err != nil {
		slog.Warn("personas: default seed failed", "err", err)
	}
	if scanner != nil && storage != nil {
		if err := reconcileFromChain(ctx, registry, scanner, storage); err != nil {
			slog.Warn("personas: chain reconcile failed", "err", err)
		}
	}
	if ensWriter != nil {
		if err := reconcileENS(ctx, registry, ensWriter); err != nil {
			slog.Warn("personas: ens reconcile failed", "err", err)
		}
	}
}

// reconcileDefaults INSERT-OR-IGNOREs the 3 builtin personas (planner, coder,
// reviewer) at their canonical token IDs (0/1/2). Idempotent: ErrPersonaNameTaken
// from a duplicate insert is swallowed.
func reconcileDefaults(ctx context.Context, registry queue.PersonaRegistry) error {
	owner := defaultOwnerAddr()
	parent := os.Getenv("PI_ENS_PARENT_NAME")

	defaults := []queue.Persona{
		{
			TokenID:         "0",
			Name:            "planner",
			OwnerAddr:       owner,
			SystemPromptURI: plannerZGURI,
			ENSSubname:      defaultSubname("planner", parent),
			Description:     "default planner — break tasks down",
		},
		{
			TokenID:         "1",
			Name:            "coder",
			OwnerAddr:       owner,
			SystemPromptURI: coderZGURI,
			ENSSubname:      defaultSubname("coder", parent),
			Description:     "default coder — Pi-in-Docker implementation",
		},
		{
			TokenID:         "2",
			Name:            "reviewer",
			OwnerAddr:       owner,
			SystemPromptURI: reviewerZGURI,
			ENSSubname:      defaultSubname("reviewer", parent),
			Description:     "default reviewer — diff critique + approve/flag",
		},
	}
	for _, p := range defaults {
		if err := registry.Insert(ctx, p); err != nil {
			if errors.Is(err, queue.ErrPersonaNameTaken) {
				continue // already present, idempotent
			}
			return fmt.Errorf("insert default %s: %w", p.Name, err)
		}
	}
	return nil
}

// reconcileFromChain scans iNFT Transfer events for tokens > maxKnownTokenID
// and imports them into the registry. The new row's name is "imported-<tokenID>"
// since on-chain we only have the URI, not the user-facing name. ens_subname
// stays empty so reconcileENS picks it up later.
func reconcileFromChain(
	ctx context.Context,
	registry queue.PersonaRegistry,
	scanner TransferScanner,
	storage queue.PromptStorage,
) error {
	known, err := registry.List(ctx)
	if err != nil {
		return fmt.Errorf("list known: %w", err)
	}
	maxKnown := int64(-1)
	for _, p := range known {
		n, perr := strconv.ParseInt(p.TokenID, 10, 64)
		if perr != nil {
			continue
		}
		if n > maxKnown {
			maxKnown = n
		}
	}

	events, err := scanner.ScanNewMints(ctx, maxKnown)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	for _, ev := range events {
		prompt, ferr := storage.FetchPrompt(ctx, ev.URI)
		if ferr != nil {
			slog.Warn("reconcile: fetch prompt failed",
				"tokenID", ev.TokenID, "uri", ev.URI, "err", ferr)
			continue
		}
		desc := prompt
		if len(desc) > 60 {
			desc = desc[:60]
		}
		row := queue.Persona{
			TokenID:         ev.TokenID,
			Name:            "imported-" + ev.TokenID,
			OwnerAddr:       ev.Owner,
			SystemPromptURI: ev.URI,
			ENSSubname:      "",
			Description:     desc,
		}
		if err := registry.Insert(ctx, row); err != nil {
			if errors.Is(err, queue.ErrPersonaNameTaken) {
				continue
			}
			slog.Warn("reconcile: insert imported persona failed",
				"tokenID", ev.TokenID, "err", err)
		}
	}
	return nil
}

// reconcileENS retries ENS subname registration for personas with empty
// ens_subname. Reuses queue.SyncPersonaENSRecords so records are byte-identical
// to those written by Queue.MintPersona at /persona-mint time. Per-persona
// failures are logged and skipped — one bad persona doesn't block the others.
func reconcileENS(ctx context.Context, registry queue.PersonaRegistry, ensWriter queue.ENSWriter) error {
	all, err := registry.List(ctx)
	if err != nil {
		return fmt.Errorf("list personas: %w", err)
	}
	inftAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
	for _, p := range all {
		if p.ENSSubname != "" {
			continue
		}
		if err := queue.SyncPersonaENSRecords(ctx, ensWriter, p, inftAddr); err != nil {
			slog.Warn("reconcile: ens sync failed", "name", p.Name, "err", err)
			continue
		}
		full := p.Name + "." + ensWriter.ParentName()
		if err := registry.UpdateENSSubname(ctx, p.Name, full); err != nil {
			slog.Warn("reconcile: persisting ens_subname failed", "name", p.Name, "err", err)
		}
	}
	return nil
}

// defaultSubname returns "<label>.<parent>" when parent is non-empty, else "".
// Empty parent means ENS env vars aren't configured; reconcileENS will fill
// the row in later if/when they are.
func defaultSubname(label, parent string) string {
	if parent == "" {
		return ""
	}
	return label + "." + parent
}

// defaultOwnerAddr returns the orchestrator's signer address derived from
// PI_ZG_PRIVATE_KEY, or "0x0000000000000000000000000000000000000000" when no
// key is set (e.g. fully offline tests). The personas table requires owner_addr
// NOT NULL, so we always return a non-empty string.
func defaultOwnerAddr() string {
	key := os.Getenv("PI_ZG_PRIVATE_KEY")
	if key == "" {
		return "0x0000000000000000000000000000000000000000"
	}
	addr, err := signerAddress(key)
	if err != nil {
		return "0x0000000000000000000000000000000000000000"
	}
	return addr
}

// signerAddress derives the Ethereum address from a hex-encoded private key.
// Used to populate personas.owner_addr for the default seed without hard-coding
// the deployer address.
func signerAddress(privKeyHex string) (string, error) {
	pk, err := crypto.HexToECDSA(strings.TrimPrefix(privKeyHex, "0x"))
	if err != nil {
		return "", err
	}
	return crypto.PubkeyToAddress(pk.PublicKey).Hex(), nil
}
