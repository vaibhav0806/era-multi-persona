// Package zg_storage provides simple blob upload + retrieval for persona
// system prompts on 0G Storage. Wraps the existing zg_kv plumbing from M7-B.
//
// Idempotent by content hash: uploading the same content twice returns the
// same URI (the underlying zg_kv keys by sha256(content)).
package zg_storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
)

// promptsNamespace is the zg_kv namespace used for persona system-prompt blobs.
const promptsNamespace = "personas/prompts"

// uriScheme is the URI prefix returned by UploadPrompt; the suffix is hex(sha256).
const uriScheme = "zg://"

// Config configures a Client backed by a real 0G storage KV node. For unit
// tests, use NewWithKV instead and pass a Provider built via zg_kv.NewWithOps.
type Config struct {
	KV zg_kv.LiveOpsConfig
}

// Client is a thin facade over zg_kv.Provider scoped to the prompts namespace.
type Client struct {
	kv     *zg_kv.Provider
	closer func()
}

// New constructs a Client backed by a live 0G KV node. Caller must Close().
func New(cfg Config) (*Client, error) {
	live, err := zg_kv.NewLiveOps(cfg.KV)
	if err != nil {
		return nil, fmt.Errorf("zg_storage: %w", err)
	}
	return &Client{
		kv:     zg_kv.NewWithOps(live),
		closer: live.Close,
	}, nil
}

// NewWithKV is a test entry point: skip live SDK construction, use the
// provided Provider directly. Caller owns the Provider's lifecycle (Close()
// on the returned Client is a no-op for the underlying ops).
func NewWithKV(kv *zg_kv.Provider) *Client {
	return &Client{kv: kv}
}

// Close releases SDK resources for clients constructed via New. Safe to call
// multiple times. No-op for clients constructed via NewWithKV.
func (c *Client) Close() {
	if c.closer != nil {
		c.closer()
		c.closer = nil
	}
}

// UploadPrompt stores content under key = sha256(content) and returns a URI
// of shape "zg://<keyhex>" pointing at it. Idempotent — same content → same URI.
func (c *Client) UploadPrompt(ctx context.Context, content string) (string, error) {
	key := contentKey(content)
	if err := c.kv.PutKV(ctx, promptsNamespace, key, []byte(content)); err != nil {
		return "", fmt.Errorf("zg_storage upload: %w", err)
	}
	return uriScheme + key, nil
}

// FetchPrompt retrieves a previously uploaded prompt by URI.
func (c *Client) FetchPrompt(ctx context.Context, uri string) (string, error) {
	if !strings.HasPrefix(uri, uriScheme) {
		return "", fmt.Errorf("zg_storage: invalid URI %q (missing %s scheme)", uri, uriScheme)
	}
	key := strings.TrimPrefix(uri, uriScheme)
	if key == "" {
		return "", errors.New("zg_storage: empty URI key")
	}
	v, err := c.kv.GetKV(ctx, promptsNamespace, key)
	if err != nil {
		return "", fmt.Errorf("zg_storage fetch: %w", err)
	}
	return string(v), nil
}

func contentKey(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
