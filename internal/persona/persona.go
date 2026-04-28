// Package persona holds the shared Persona row type and sentinel errors
// used by both internal/db (the store) and internal/queue (the consumer).
// It exists solely to break what would otherwise be an internal/db ↔
// internal/queue import cycle: the queue package defines the consumer
// interface (PersonaRegistry), the db package implements the methods, and
// both reference the same struct + errors from this neutral package.
package persona

import (
	"errors"
	"time"
)

// Persona is the local-DB row for a minted PersonaNFT. The registry contract
// on-chain is the source of truth; this is era's cached view used to resolve
// /mention <name> → token_id at task-creation time.
type Persona struct {
	TokenID         string
	Name            string
	OwnerAddr       string
	SystemPromptURI string
	ENSSubname      string
	Description     string
	PromptText      string
	CreatedAt       time.Time
}

// Sentinel errors returned by Repo persona methods (and re-exported as
// queue.ErrPersonaNotFound / queue.ErrPersonaNameTaken).
var (
	ErrPersonaNotFound  = errors.New("persona not found")
	ErrPersonaNameTaken = errors.New("persona name already taken")
)
