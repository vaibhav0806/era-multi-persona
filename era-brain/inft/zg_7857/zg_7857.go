// Package zg_7857 is an inft.Registry impl wrapping abigen bindings for the
// EraPersonaINFT contract deployed on 0G Galileo testnet.
//
// M7-D.2 scope: only RecordInvocation is implemented. Mint and Lookup return
// ErrNotImplemented (deferred to M7-D.3+).
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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/vaibhav0806/era-multi-persona/era-brain/inft"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857/bindings"
)

var ErrNotImplemented = errors.New("zg_7857: not implemented in M7-D.2 (deferred)")

type Config struct {
	ContractAddress string
	EVMRPCURL       string
	PrivateKey      string
	ChainID         int64
}

type Provider struct {
	cfg      Config
	client   *ethclient.Client
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

	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("zg_7857 priv key: %w", err)
	}

	contract, err := bindings.NewEraPersonaINFT(common.HexToAddress(cfg.ContractAddress), client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("zg_7857 bind contract: %w", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privKey, big.NewInt(cfg.ChainID))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("zg_7857 auth: %w", err)
	}

	return &Provider{cfg: cfg, client: client, contract: contract, auth: auth, privKey: privKey}, nil
}

func (p *Provider) Close() {
	if p.client != nil {
		p.client.Close()
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

func (p *Provider) Mint(_ context.Context, _, _ string) (inft.Persona, error) {
	return inft.Persona{}, ErrNotImplemented
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
