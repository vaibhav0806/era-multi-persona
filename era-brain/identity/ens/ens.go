// Package ens is an identity.Resolver impl wrapping abigen bindings for the
// ENS NameWrapper + PublicResolver contracts on Sepolia. ABIs come from
// minimal mock contracts under contracts/test/ but match the real Sepolia
// contracts' subset of methods we use, so the same Go code works against
// both simulated.Backend (unit tests) and the real Sepolia chain (live test
// + production).
package ens

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/vaibhav0806/era-multi-persona/era-brain/identity"
	"github.com/vaibhav0806/era-multi-persona/era-brain/identity/ens/bindings"
)

const (
	SepoliaNameWrapper    = "0x0635513f179D50A207757E05759CbD106d7dFcE8"
	SepoliaPublicResolver = "0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5"
)

type Config struct {
	ParentName string
	RPCURL     string
	PrivateKey string
	ChainID    int64

	// Optional address overrides for tests. Leave empty in production.
	NameWrapperAddress string
	ResolverAddress    string
}

type ContractClient interface {
	bind.ContractBackend
	bind.DeployBackend
}

type Provider struct {
	cfg        Config
	parentNode [32]byte

	client       ContractClient
	dialedClient *ethclient.Client

	nameWrapper *bindings.NameWrapper
	resolver    *bindings.PublicResolver
	auth        *bind.TransactOpts
	signer      common.Address
}

var _ identity.Resolver = (*Provider)(nil)

func New(cfg Config) (*Provider, error) {
	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("ens dial: %w", err)
	}
	p, err := newWithBackend(cfg, client)
	if err != nil {
		client.Close()
		return nil, err
	}
	p.dialedClient = client
	return p, nil
}

func NewWithClient(cfg Config, client ContractClient) (*Provider, error) {
	return newWithBackend(cfg, client)
}

func newWithBackend(cfg Config, client ContractClient) (*Provider, error) {
	parentNode, err := Namehash(cfg.ParentName)
	if err != nil {
		return nil, fmt.Errorf("ens namehash parent: %w", err)
	}

	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("ens priv key: %w", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privKey, big.NewInt(cfg.ChainID))
	if err != nil {
		return nil, fmt.Errorf("ens auth: %w", err)
	}

	nwAddrHex := cfg.NameWrapperAddress
	if nwAddrHex == "" {
		nwAddrHex = SepoliaNameWrapper
	}
	resAddrHex := cfg.ResolverAddress
	if resAddrHex == "" {
		resAddrHex = SepoliaPublicResolver
	}

	nw, err := bindings.NewNameWrapper(common.HexToAddress(nwAddrHex), client)
	if err != nil {
		return nil, fmt.Errorf("ens bind name_wrapper: %w", err)
	}
	res, err := bindings.NewPublicResolver(common.HexToAddress(resAddrHex), client)
	if err != nil {
		return nil, fmt.Errorf("ens bind resolver: %w", err)
	}

	signer := pubkeyAddr(privKey)

	return &Provider{
		cfg:         cfg,
		parentNode:  parentNode,
		client:      client,
		nameWrapper: nw,
		resolver:    res,
		auth:        auth,
		signer:      signer,
	}, nil
}

func pubkeyAddr(k *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(k.PublicKey)
}

func (p *Provider) Close() {
	if p.dialedClient != nil {
		p.dialedClient.Close()
	}
}

func (p *Provider) ParentName() string { return p.cfg.ParentName }

// EnsureSubname registers <label>.<parent> if not already owned by the signer.
// Idempotent: returns nil without sending a tx when the subnode already
// resolves to the signer in NameWrapper.
//
// Calls NameWrapper.setSubnodeRecord with expiry = max-uint64 (passing 0
// reverts on Sepolia NameWrapper when the parent has any fuses burned;
// max-uint64 lets the contract clamp to the parent's expiry internally).
func (p *Provider) EnsureSubname(ctx context.Context, label string) error {
	subnode, err := p.subnameNode(label)
	if err != nil {
		return err
	}
	tokenID := new(big.Int).SetBytes(subnode[:])

	owner, err := p.nameWrapper.OwnerOf(&bind.CallOpts{Context: ctx}, tokenID)
	if err != nil {
		return fmt.Errorf("ens ownerOf %s: %w", label, err)
	}
	if owner == p.signer {
		return nil
	}

	auth := *p.auth
	auth.Context = ctx

	resAddrHex := p.cfg.ResolverAddress
	if resAddrHex == "" {
		resAddrHex = SepoliaPublicResolver
	}
	tx, err := p.nameWrapper.SetSubnodeRecord(
		&auth,
		p.parentNode,
		label,
		p.signer,
		common.HexToAddress(resAddrHex),
		uint64(0),  // ttl
		uint32(0),  // fuses
		^uint64(0), // expiry — sentinel "use parent's"; NameWrapper clamps internally
	)
	if err != nil {
		return fmt.Errorf("ens setSubnodeRecord %s: %w", label, err)
	}
	if err := p.waitMined(ctx, tx, "setSubnodeRecord "+label); err != nil {
		return err
	}
	return nil
}

func (p *Provider) SetTextRecord(ctx context.Context, label, key, value string) error {
	subnode, err := p.subnameNode(label)
	if err != nil {
		return err
	}
	current, err := p.resolver.Text(&bind.CallOpts{Context: ctx}, subnode, key)
	if err != nil {
		return fmt.Errorf("ens read text %s.%s: %w", label, key, err)
	}
	if current == value {
		return nil
	}

	auth := *p.auth
	auth.Context = ctx
	tx, err := p.resolver.SetText(&auth, subnode, key, value)
	if err != nil {
		return fmt.Errorf("ens setText %s.%s: %w", label, key, err)
	}
	if err := p.waitMined(ctx, tx, fmt.Sprintf("setText %s.%s", label, key)); err != nil {
		return err
	}
	return nil
}

func (p *Provider) ReadTextRecord(ctx context.Context, label, key string) (string, error) {
	subnode, err := p.subnameNode(label)
	if err != nil {
		return "", err
	}
	v, err := p.resolver.Text(&bind.CallOpts{Context: ctx}, subnode, key)
	if err != nil {
		return "", fmt.Errorf("ens read text %s.%s: %w", label, key, err)
	}
	return v, nil
}

// waitMined blocks until the tx is mined, then verifies the receipt status.
// Only runs when this Provider was constructed via New() against a real RPC
// (ethclient.Dial). When constructed via NewWithClient (e.g. simulated.Backend
// in unit tests), the test driver controls block production explicitly, so
// blocking here would deadlock — we skip the wait and let the caller Commit.
func (p *Provider) waitMined(ctx context.Context, tx *types.Transaction, label string) error {
	if tx == nil || p.dialedClient == nil {
		return nil
	}
	receipt, err := bind.WaitMined(ctx, p.dialedClient, tx)
	if err != nil {
		return fmt.Errorf("ens wait %s: %w", label, err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("ens %s tx %s reverted on-chain", label, tx.Hash().Hex())
	}
	return nil
}

func (p *Provider) subnameNode(label string) ([32]byte, error) {
	if label == "" {
		return [32]byte{}, errors.New("ens: empty label")
	}
	return Namehash(label + "." + p.cfg.ParentName)
}

// Namehash computes the ENS namehash of `name` per ENSIP-1.
// Empty string → bytes32(0). Otherwise recursive keccak256 of (parent || keccak256(label)).
func Namehash(name string) ([32]byte, error) {
	var node [32]byte
	if name == "" {
		return node, nil
	}
	labels := strings.Split(name, ".")
	for i := len(labels) - 1; i >= 0; i-- {
		if labels[i] == "" {
			return node, fmt.Errorf("ens: empty label in %q", name)
		}
		labelHash := crypto.Keccak256([]byte(labels[i]))
		concat := append(node[:], labelHash...)
		next := crypto.Keccak256(concat)
		copy(node[:], next)
	}
	return node, nil
}
