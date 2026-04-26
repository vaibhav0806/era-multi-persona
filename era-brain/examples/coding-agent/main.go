// coding-agent demonstrates the era-brain 3-persona flow against a synthetic task.
//
// Usage:
//
//	OPENROUTER_API_KEY=sk-... go run ./examples/coding-agent --task="add JWT auth to /login endpoint"
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
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite"
)

func main() {
	task := flag.String("task", "", "task description (required)")
	model := flag.String("model", "openai/gpt-4o-mini", "OpenRouter model id")
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

	llmProv := openrouter.New(openrouter.Config{
		APIKey:       apiKey,
		DefaultModel: *model,
	})

	personas := []brain.Persona{
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "planner",
			SystemPrompt: plannerSystemPrompt,
			LLM:          llmProv,
			Memory:       mem,
			Now:          time.Now,
		}),
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "coder",
			SystemPrompt: coderSystemPrompt,
			LLM:          llmProv,
			Memory:       mem,
			Now:          time.Now,
		}),
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "reviewer",
			SystemPrompt: reviewerSystemPrompt,
			LLM:          llmProv,
			Memory:       mem,
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
