package brain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// Persona is one stage in a Brain run. It receives the threaded conversation
// state, produces output, and writes a Receipt. Impls choose how to use the
// underlying LLMProvider and MemoryProvider; brain only orchestrates the chain.
type Persona interface {
	Name() string
	Run(ctx context.Context, in Input) (Output, error)
}

// Input threads task context through the persona chain.
type Input struct {
	TaskID          string
	UserID          string
	TaskDescription string
	PriorOutputs    []Output
}

// Output is what a persona emits.
type Output struct {
	PersonaName string
	Text        string
	Receipt     Receipt
}

// LLMPersonaConfig configures a concrete LLM-backed Persona.
type LLMPersonaConfig struct {
	Name         string
	SystemPrompt string
	Model        string // passed as Request.Model; empty = LLM provider default
	LLM          llm.Provider
	Memory       memory.Provider // used for audit-log writes; KV reads land in M7-B
	Now          func() time.Time
}

// LLMPersona is the standard Persona impl: builds a prompt from config +
// PriorOutputs, calls the LLM, computes the receipt's input/output hashes
// from the prompt and response (so impls don't have to), writes a receipt
// to the audit log.
type LLMPersona struct {
	cfg LLMPersonaConfig
}

// NewLLMPersona constructs an LLMPersona. Now defaults to time.Now.
func NewLLMPersona(cfg LLMPersonaConfig) *LLMPersona {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &LLMPersona{cfg: cfg}
}

func (p *LLMPersona) Name() string { return p.cfg.Name }

func (p *LLMPersona) Run(ctx context.Context, in Input) (Output, error) {
	user := buildUserPrompt(in)
	resp, err := p.cfg.LLM.Complete(ctx, llm.Request{
		SystemPrompt: p.cfg.SystemPrompt,
		UserPrompt:   user,
		Model:        p.cfg.Model,
	})
	if err != nil {
		return Output{}, fmt.Errorf("llm complete: %w", err)
	}
	// Hash the *requested* model (cfg.Model, possibly empty) for receipt
	// determinism. The upstream API may substitute a different model; we
	// record what we asked for so receipts collide on identical requests.
	inH := sha256Hex(p.cfg.SystemPrompt + "\x00" + user + "\x00" + p.cfg.Model)
	outH := sha256Hex(resp.Text)
	r := Receipt{
		Persona:       p.cfg.Name,
		Model:         resp.Model,
		InputHash:     inH,
		OutputHash:    outH,
		Sealed:        resp.Sealed,
		TimestampUnix: p.cfg.Now().Unix(),
	}
	if p.cfg.Memory != nil && in.TaskID != "" {
		entry, _ := json.Marshal(r)
		_ = p.cfg.Memory.AppendLog(ctx, "audit/"+in.TaskID, entry)
	}
	return Output{PersonaName: p.cfg.Name, Text: resp.Text, Receipt: r}, nil
}

func buildUserPrompt(in Input) string {
	var b strings.Builder
	if in.TaskDescription != "" {
		b.WriteString("Task: ")
		b.WriteString(in.TaskDescription)
		b.WriteString("\n\n")
	}
	for _, o := range in.PriorOutputs {
		b.WriteString("--- ")
		b.WriteString(o.PersonaName)
		b.WriteString(" output ---\n")
		b.WriteString(o.Text)
		b.WriteString("\n\n")
	}
	return b.String()
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
