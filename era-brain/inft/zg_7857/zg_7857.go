// Package zg_7857 is an inft.Registry impl wrapping abigen bindings for the
// EraPersonaINFT contract deployed on 0G Galileo testnet.
package zg_7857

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/vaibhav0806/era-multi-persona/era-brain/inft"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857/bindings"
)

var ErrNotImplemented = errors.New("zg_7857: not implemented")

type Config struct {
	ContractAddress string
	EVMRPCURL       string
	PrivateKey      string
	ChainID         int64
}

// ContractClient is the subset of *ethclient.Client + simulated.Client we need.
// Both satisfy bind.ContractBackend and bind.DeployBackend; in tests we pass a
// simulated.Client, in prod a *ethclient.Client.
type ContractClient interface {
	bind.ContractBackend
	bind.DeployBackend
}

type Provider struct {
	cfg Config

	client       ContractClient
	dialedClient *ethclient.Client

	contract *bindings.EraPersonaINFT
	auth     *bind.TransactOpts
	privKey  *ecdsa.PrivateKey
}

var _ inft.Registry = (*Provider)(nil)

func New(cfg Config) (*Provider, error) {
	client, err := ethclient.Dial(cfg.EVMRPCURL)
	if err != nil {
		return nil, fmt.Errorf("zg_7857 dial: %w", err)
	}
	p, err := newWithBackend(cfg, client)
	if err != nil {
		client.Close()
		return nil, err
	}
	p.dialedClient = client
	return p, nil
}

// NewWithClient is a test entry point: skip dial, use the provided client.
// Production callers use New.
func NewWithClient(cfg Config, client ContractClient) (*Provider, error) {
	return newWithBackend(cfg, client)
}

func newWithBackend(cfg Config, client ContractClient) (*Provider, error) {
	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("zg_7857 priv key: %w", err)
	}

	contract, err := bindings.NewEraPersonaINFT(common.HexToAddress(cfg.ContractAddress), client)
	if err != nil {
		return nil, fmt.Errorf("zg_7857 bind contract: %w", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privKey, big.NewInt(cfg.ChainID))
	if err != nil {
		return nil, fmt.Errorf("zg_7857 auth: %w", err)
	}

	return &Provider{cfg: cfg, client: client, contract: contract, auth: auth, privKey: privKey}, nil
}

func (p *Provider) Close() {
	if p.dialedClient != nil {
		p.dialedClient.Close()
	}
}

func (p *Provider) RecordInvocation(ctx context.Context, tokenID string, receiptHashHex string) error {
	tokenIDBig, ok := new(big.Int).SetString(tokenID, 10)
	if !ok {
		return fmt.Errorf("zg_7857 invalid tokenID %q", tokenID)
	}
	hash, err := DecodeReceiptHash(receiptHashHex)
	if err != nil {
		return fmt.Errorf("zg_7857 decode receiptHash: %w", err)
	}

	// Shallow copy of auth so we can override Context per call. Signer (pointer)
	// stays shared — fine because era's queue serializes tasks today.
	auth := *p.auth
	auth.Context = ctx

	if _, err := p.contract.RecordInvocation(&auth, tokenIDBig, hash); err != nil {
		return fmt.Errorf("zg_7857 recordInvocation tx: %w", err)
	}
	return nil
}

// Mint creates a new persona iNFT token, owned by the orchestrator wallet.
// systemPromptURI becomes the contract's tokenURI for the new token.
// Returns the auto-incremented token ID + persona metadata.
//
// Calls EraPersonaINFT.mint(address to, string memory uri) — onlyOwner.
// Parses the Transfer(from=0x0, to=signer, tokenId) event from the receipt
// to extract the auto-incremented token ID.
func (p *Provider) Mint(ctx context.Context, name, systemPromptURI string) (inft.Persona, error) {
	auth := *p.auth
	auth.Context = ctx

	tx, err := p.contract.Mint(&auth, p.auth.From, systemPromptURI)
	if err != nil {
		return inft.Persona{}, fmt.Errorf("zg_7857 mint tx: %w", err)
	}

	rc, err := bind.WaitMined(ctx, p.client, tx)
	if err != nil {
		return inft.Persona{}, fmt.Errorf("zg_7857 mint waitmined: %w", err)
	}
	if rc.Status != types.ReceiptStatusSuccessful {
		return inft.Persona{}, fmt.Errorf("zg_7857 mint reverted: txHash=%s", tx.Hash().Hex())
	}

	// Find Transfer(from=0x0, to=signer) in receipt logs.
	var tokenID *big.Int
	for _, log := range rc.Logs {
		ev, perr := p.contract.ParseTransfer(*log)
		if perr != nil {
			continue // not a Transfer event
		}
		zero := common.Address{}
		if ev.From == zero && ev.To == p.auth.From {
			tokenID = ev.TokenId
			break
		}
	}
	if tokenID == nil {
		return inft.Persona{}, fmt.Errorf("zg_7857 mint: no matching Transfer event in receipt; txHash=%s", tx.Hash().Hex())
	}

	return inft.Persona{
		TokenID:         tokenID.String(),
		Name:            name,
		SystemPromptURI: systemPromptURI,
		OwnerAddr:       p.auth.From.Hex(),
		ContractAddr:    p.cfg.ContractAddress,
		MintTxHash:      tx.Hash().Hex(),
	}, nil
}

func (p *Provider) Lookup(_ context.Context, _, _ string) (inft.Persona, error) {
	return inft.Persona{}, ErrNotImplemented
}

func DecodeReceiptHash(hexStr string) ([32]byte, error) {
	var hash [32]byte
	raw, err := hex.DecodeString(strings.TrimPrefix(hexStr, "0x"))
	if err != nil {
		return hash, fmt.Errorf("hex decode: %w", err)
	}
	if len(raw) != 32 {
		return hash, fmt.Errorf("expected 32 bytes, got %d", len(raw))
	}
	copy(hash[:], raw)
	return hash, nil
}
