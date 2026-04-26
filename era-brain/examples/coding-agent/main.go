// coding-agent demonstrates the era-brain 3-persona flow against a synthetic task.
//
// Usage:
//
//	OPENROUTER_API_KEY=sk-... go run ./examples/coding-agent --task="add JWT auth to /login endpoint"
//
// For 0G testnet writes alongside SQLite:
//
//	PI_ZG_PRIVATE_KEY=0x... PI_ZG_EVM_RPC=... PI_ZG_INDEXER_RPC=... \
//	  go run ./examples/coding-agent --task="..." --zg-live
//
// Output: planner plan, coder diff, reviewer critique + decision, plus per-persona receipts.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/openrouter"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/dual"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
)

func main() {
	task := flag.String("task", "", "task description (required)")
	model := flag.String("model", "openai/gpt-4o-mini", "OpenRouter model id")
	zgLive := flag.Bool("zg-live", false, "use 0G testnet alongside SQLite (requires PI_ZG_* env vars)")
	flag.Parse()
	if *task == "" {
		log.Fatal("--task is required")
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		// Fallback to era's existing convention: PI_OPENROUTER_API_KEY.
		// Lets the example work directly out of era's .env without a separate alias.
		apiKey = os.Getenv("PI_OPENROUTER_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY (or PI_OPENROUTER_API_KEY) is required")
	}

	dbPath := filepath.Join(os.TempDir(), "era-brain-example.db")
	mem, err := sqlite.Open(dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer mem.Close()
	fmt.Fprintf(os.Stderr, "[memory db: %s]\n", dbPath)

	var memProv memory.Provider = mem
	if *zgLive {
		priv := os.Getenv("PI_ZG_PRIVATE_KEY")
		rpc := os.Getenv("PI_ZG_EVM_RPC")
		indexer := os.Getenv("PI_ZG_INDEXER_RPC")
		kvNode := os.Getenv("PI_ZG_KV_NODE") // optional
		if priv == "" || rpc == "" || indexer == "" {
			log.Fatal("--zg-live set but PI_ZG_PRIVATE_KEY/PI_ZG_EVM_RPC/PI_ZG_INDEXER_RPC missing")
		}
		live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
			PrivateKey: priv, EVMRPCURL: rpc, IndexerURL: indexer, KVNodeURL: kvNode,
		})
		if err != nil {
			log.Fatalf("zg live ops: %v", err)
		}
		defer live.Close()

		primary := &composite{
			kvP:  zg_kv.NewWithOps(live),
			logP: zg_log.NewWithOps(live),
		}
		memProv = dual.New(mem, primary, func(op string, err error) {
			fmt.Fprintf(os.Stderr, "[zg primary %s failed: %v]\n", op, err)
		})
	}

	llmProv := openrouter.New(openrouter.Config{
		APIKey:       apiKey,
		DefaultModel: *model,
	})

	personas := []brain.Persona{
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "planner",
			SystemPrompt: plannerSystemPrompt,
			LLM:          llmProv,
			Memory:       memProv,
			Now:          time.Now,
		}),
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "coder",
			SystemPrompt: coderSystemPrompt,
			LLM:          llmProv,
			Memory:       memProv,
			Now:          time.Now,
		}),
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "reviewer",
			SystemPrompt: reviewerSystemPrompt,
			LLM:          llmProv,
			Memory:       memProv,
			Now:          time.Now,
		}),
	}

	b := brain.New()
	taskID := fmt.Sprintf("example-%d", time.Now().Unix())
	res, err := b.Run(context.Background(), brain.Input{
		TaskID:          taskID,
		UserID:          "local",
		TaskDescription: *task,
	}, personas)
	if err != nil {
		log.Fatalf("brain run: %v", err)
	}

	for _, o := range res.Outputs {
		fmt.Printf("\n========== %s ==========\n", o.PersonaName)
		fmt.Println(o.Text)
		fmt.Printf("\n[receipt: model=%s sealed=%t hash=%s]\n",
			o.Receipt.Model, o.Receipt.Sealed, brain.ReceiptHash(o.Receipt))
	}
}

// composite combines a KV-only provider and a Log-only provider into a single
// memory.Provider for cases where they're backed by the same underlying
// transport (here: shared zg_kv.LiveOps for both).
type composite struct {
	kvP  memory.Provider
	logP memory.Provider
}

func (c *composite) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
	return c.kvP.GetKV(ctx, ns, key)
}
func (c *composite) PutKV(ctx context.Context, ns, key string, val []byte) error {
	return c.kvP.PutKV(ctx, ns, key, val)
}
func (c *composite) AppendLog(ctx context.Context, ns string, entry []byte) error {
	return c.logP.AppendLog(ctx, ns, entry)
}
func (c *composite) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
	return c.logP.ReadLog(ctx, ns)
}
