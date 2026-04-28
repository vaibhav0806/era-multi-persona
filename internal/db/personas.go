package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/vaibhav0806/era/internal/persona"
)

// Persona table CRUD. Uses raw SQL via the sqlc-generated *Queries' embedded
// DBTX rather than going through sqlc — keeps the migration self-contained
// for M7-F without regenerating sqlc bindings for a small handful of queries.

const insertPersonaSQL = `INSERT INTO personas (token_id, name, owner_addr, system_prompt_uri, ens_subname, description, prompt_text) VALUES (?, ?, ?, ?, ?, ?, ?)`

const getPersonaByNameSQL = `SELECT token_id, name, owner_addr, system_prompt_uri, ens_subname, description, created_at FROM personas WHERE name = ?`

const listPersonasSQL = `SELECT token_id, name, owner_addr, system_prompt_uri, ens_subname, description, created_at FROM personas ORDER BY CAST(token_id AS INTEGER) ASC`

const getPersonaPromptSQL = `SELECT prompt_text FROM personas WHERE name = ?`

const updatePersonaENSSubnameSQL = `UPDATE personas SET ens_subname = ? WHERE name = ?`

func (r *Repo) InsertPersona(ctx context.Context, p persona.Persona) error {
	_, err := r.q.db.ExecContext(ctx, insertPersonaSQL,
		p.TokenID,
		p.Name,
		p.OwnerAddr,
		p.SystemPromptURI,
		nullableString(p.ENSSubname),
		nullableString(p.Description),
		p.PromptText,
	)
	if err != nil {
		if isUniqueViolation(err, "personas.name") {
			return persona.ErrPersonaNameTaken
		}
		return fmt.Errorf("insert persona: %w", err)
	}
	return nil
}

func (r *Repo) GetPersonaByName(ctx context.Context, name string) (persona.Persona, error) {
	row := r.q.db.QueryRowContext(ctx, getPersonaByNameSQL, name)
	var (
		p          persona.Persona
		ensSubname sql.NullString
		desc       sql.NullString
	)
	if err := row.Scan(
		&p.TokenID,
		&p.Name,
		&p.OwnerAddr,
		&p.SystemPromptURI,
		&ensSubname,
		&desc,
		&p.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return persona.Persona{}, persona.ErrPersonaNotFound
		}
		return persona.Persona{}, fmt.Errorf("get persona by name: %w", err)
	}
	p.ENSSubname = ensSubname.String
	p.Description = desc.String
	return p, nil
}

func (r *Repo) ListPersonas(ctx context.Context) ([]persona.Persona, error) {
	rows, err := r.q.db.QueryContext(ctx, listPersonasSQL)
	if err != nil {
		return nil, fmt.Errorf("list personas: %w", err)
	}
	defer rows.Close()

	var out []persona.Persona
	for rows.Next() {
		var (
			p          persona.Persona
			ensSubname sql.NullString
			desc       sql.NullString
		)
		if err := rows.Scan(
			&p.TokenID,
			&p.Name,
			&p.OwnerAddr,
			&p.SystemPromptURI,
			&ensSubname,
			&desc,
			&p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan persona: %w", err)
		}
		p.ENSSubname = ensSubname.String
		p.Description = desc.String
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate personas: %w", err)
	}
	return out, nil
}

// GetPersonaPrompt returns the cached system-prompt body for a persona by name.
// Returns persona.ErrPersonaNotFound if no row matches. An empty prompt_text on
// an existing row is a valid result and returned as ("", nil) — callers that
// want fallback to fetching from SystemPromptURI must check the empty string.
func (r *Repo) GetPersonaPrompt(ctx context.Context, name string) (string, error) {
	row := r.q.db.QueryRowContext(ctx, getPersonaPromptSQL, name)
	var prompt string
	if err := row.Scan(&prompt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", persona.ErrPersonaNotFound
		}
		return "", fmt.Errorf("get persona prompt: %w", err)
	}
	return prompt, nil
}

func (r *Repo) UpdatePersonaENSSubname(ctx context.Context, name, subname string) error {
	_, err := r.q.db.ExecContext(ctx, updatePersonaENSSubnameSQL, nullableString(subname), name)
	if err != nil {
		return fmt.Errorf("update persona ens_subname: %w", err)
	}
	return nil
}

// PersonaRegistry adapter methods — short aliases that match queue.PersonaRegistry.
// The verbose names above are kept for direct use elsewhere in era.

func (r *Repo) Lookup(ctx context.Context, name string) (persona.Persona, error) {
	return r.GetPersonaByName(ctx, name)
}

func (r *Repo) Insert(ctx context.Context, p persona.Persona) error {
	return r.InsertPersona(ctx, p)
}

func (r *Repo) List(ctx context.Context) ([]persona.Persona, error) {
	return r.ListPersonas(ctx)
}

func (r *Repo) UpdateENSSubname(ctx context.Context, name, subname string) error {
	return r.UpdatePersonaENSSubname(ctx, name, subname)
}

// nullableString returns sql.NullString{Valid:false} for "" so empty strings
// land as NULL in nullable columns (ens_subname, description).
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// isUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure
// matching the given column hint (e.g. "personas.name"). modernc.org/sqlite
// surfaces these as errors whose Error() contains "UNIQUE constraint failed: <col>".
func isUniqueViolation(err error, hint string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") && strings.Contains(msg, hint)
}
