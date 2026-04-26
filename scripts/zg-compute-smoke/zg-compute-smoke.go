// zg-compute-smoke is a standalone 0G Compute SDK verification script. Run with:
//
//	set -a; source .env; set +a
//	go run ./scripts/zg-compute-smoke
//
// Sends one bearer-auth POST to the configured 0G Compute provider, prints the
// response model + first 100 chars of content + which response headers are
// present. Use this output to identify the TEE-signature header name (e.g.
// `ZG-Res-Key` or whatever the actual provider uses) so M7-C.1.1's
// zg_compute.Provider can detect Sealed=true correctly.
//
// Phase 0 success = "OK" prints AND a TEE-signature-shaped header is visible
// in the dumped response headers.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model    string    `json:"model"`
	Messages []chatMsg `json:"messages"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
}

func main() {
	endpoint := os.Getenv("PI_ZG_COMPUTE_ENDPOINT")
	bearer := os.Getenv("PI_ZG_COMPUTE_BEARER")
	model := os.Getenv("PI_ZG_COMPUTE_MODEL")
	if endpoint == "" || bearer == "" || model == "" {
		log.Fatal("PI_ZG_COMPUTE_ENDPOINT, PI_ZG_COMPUTE_BEARER, PI_ZG_COMPUTE_MODEL required")
	}

	body := chatReq{
		Model: model,
		Messages: []chatMsg{
			{Role: "system", Content: "You are a one-line answerer."},
			{Role: "user", Content: "What is 2 + 2?"},
		},
	}
	buf, _ := json.Marshal(body)

	url := strings.TrimRight(endpoint, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		log.Fatalf("build req: %v", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+bearer)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("[status] %d\n", resp.StatusCode)
	fmt.Println("[response headers]")
	for k, v := range resp.Header {
		fmt.Printf("  %s: %s\n", k, strings.Join(v, "; "))
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("provider error: %s", string(respBody))
	}

	var parsed chatResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		log.Fatalf("parse: %v; body=%s", err, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		log.Fatalf("no choices: %s", string(respBody))
	}

	text := parsed.Choices[0].Message.Content
	if len(text) > 100 {
		text = text[:100] + "..."
	}
	fmt.Printf("\n[model]   %s\n[content] %s\n", parsed.Model, text)
	fmt.Println("\nOK")
	fmt.Println("\nNext steps:")
	fmt.Println("- Identify the TEE-signature response header from the dump above")
	fmt.Println("  (likely 'ZG-Res-Key', 'ZG-Signature', or similar)")
	fmt.Println("- Use that header name in M7-C.1.1's zg_compute.Provider.Complete")
}
