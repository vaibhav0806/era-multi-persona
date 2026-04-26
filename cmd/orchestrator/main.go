package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"path/filepath"

	"github.com/joho/godotenv"
	brainsqlite "github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/dual"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/fallback"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/openrouter"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/zg_compute"
	"github.com/vaibhav0806/era-multi-persona/era-brain/identity/ens"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857"
	"github.com/vaibhav0806/era/internal/config"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/diffscan"
	"github.com/vaibhav0806/era/internal/digest"
	"github.com/vaibhav0806/era/internal/githubapp"
	"github.com/vaibhav0806/era/internal/githubbranch"
	"github.com/vaibhav0806/era/internal/githubcompare"
	"github.com/vaibhav0806/era/internal/githubpr"
	"github.com/vaibhav0806/era/internal/queue"
	"github.com/vaibhav0806/era/internal/runner"
	"github.com/vaibhav0806/era/internal/swarm"
	"github.com/vaibhav0806/era/internal/telegram"
)

// Defense-in-depth secret scrubbing at the Telegram boundary. The runner
// already scrubs in cmd/runner/git.go; this catches anything that slips past.
var (
	tokenizedURLPat = regexp.MustCompile(`(https://x-access-token:)[^@]+@`)
	classicPATPat   = regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`)
	finePATPat      = regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)
)

func scrubSecrets(s string) string {
	s = tokenizedURLPat.ReplaceAllString(s, "$1***@")
	s = classicPATPat.ReplaceAllString(s, "ghp_***")
	s = finePATPat.ReplaceAllString(s, "github_pat_***")
	return s
}

var version = "0.0.1-m0"

// pollInterval is how often the orchestrator checks for queued tasks.
// 2s is short enough to feel responsive and long enough to stay cheap.
const pollInterval = 2 * time.Second

// Persona metadata zg-storage URIs (raw GitHub blobs of the JSON files at
// contracts/metadata/{planner,coder,reviewer}.json). Hardcoded — they only
// change when the metadata schema evolves.
const (
	plannerZGURI  = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/planner.json"
	coderZGURI    = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/coder.json"
	reviewerZGURI = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/reviewer.json"
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Info(".env not loaded", "err", err)
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handle, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		fail(err)
	}
	defer handle.Close()
	repo := db.NewRepo(handle)

	docker := &runner.Docker{
		Image:            "era-runner:m2",
		SandboxRepo:      cfg.GitHubSandboxRepo,
		OpenRouterAPIKey: cfg.OpenRouterAPIKey,
		PiModel:          cfg.PiModel,
		MaxTokens:        cfg.MaxTokensPerTask,
		MaxCostCents:     cfg.MaxCostCentsPerTask,
		MaxIterations:    cfg.MaxIterationsPerTask,
		MaxWallSeconds:   cfg.MaxWallClockSeconds,
	}

	appClient, err := githubapp.New(githubapp.Config{
		AppID:            cfg.GitHubAppID,
		InstallationID:   cfg.GitHubAppInstallationID,
		PrivateKeyBase64: cfg.GitHubAppPrivateKeyBase64,
	})
	if err != nil {
		fail(fmt.Errorf("github app init: %w", err))
	}
	var tokenSource queue.TokenSource = appClient
	slog.Info("github app token source configured",
		"app_id", cfg.GitHubAppID, "installation_id", cfg.GitHubAppInstallationID)

	compareClient := githubcompare.New("", appClient)
	branchDeleter := githubbranch.New("", appClient)
	prClient := githubpr.New("", appClient)
	ra := &runner.QueueAdapter{D: docker}
	q := queue.New(repo, ra, tokenSource, compareClient, cfg.GitHubSandboxRepo)
	ra.SetRunning(q.Running())
	q.SetBranchDeleter(branchDeleter)
	q.SetPRCreator(prClient)
	q.SetKiller(queue.NewDockerKiller())

	// Task 3: wire era-brain swarm (planner + reviewer personas).
	plannerModel := envOrDefault("PI_BRAIN_PLANNER_MODEL", "openai/gpt-4o-mini")
	reviewerModel := envOrDefault("PI_BRAIN_REVIEWER_MODEL", "openai/gpt-4o-mini")
	plannerOR := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: plannerModel})
	reviewerOR := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: reviewerModel})
	brainDBPath := filepath.Join(filepath.Dir(cfg.DBPath), "era-brain.db")
	brainMem, err := brainsqlite.Open(brainDBPath)
	if err != nil {
		fail(fmt.Errorf("era-brain sqlite: %w", err))
	}
	defer brainMem.Close()

	// Build the memory provider passed to swarm. Default = sqlite alone (M7-A.5 behavior).
	// If 0G testnet env vars are present, wrap sqlite with the dual provider so audit
	// log writes land on BOTH 0G AND SQLite.
	var memProv memory.Provider = brainMem
	if zgEnabled() {
		live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
			PrivateKey: os.Getenv("PI_ZG_PRIVATE_KEY"),
			EVMRPCURL:  os.Getenv("PI_ZG_EVM_RPC"),
			IndexerURL: os.Getenv("PI_ZG_INDEXER_RPC"),
			KVNodeURL:  os.Getenv("PI_ZG_KV_NODE"), // optional
		})
		if err != nil {
			fail(fmt.Errorf("0G live ops: %w", err))
		}
		defer live.Close()
		primary := &zgComposite{
			kvP:  zg_kv.NewWithOps(live),
			logP: zg_log.NewWithOps(live),
		}
		memProv = dual.New(brainMem, primary, func(op string, err error) {
			slog.Warn("0G primary write failed", "op", op, "err", err)
		})
		slog.Info("0G storage wired",
			"indexer", os.Getenv("PI_ZG_INDEXER_RPC"),
			"kv_node_set", os.Getenv("PI_ZG_KV_NODE") != "")
	}

	// Build the LLM providers passed to swarm. Default = OpenRouter alone (M7-B.3 baseline).
	// If 0G Compute env vars are present, wrap each persona's LLM with fallback so
	// inference tries 0G Compute first, falls back to OpenRouter on error.
	var plannerLLM llm.Provider = plannerOR
	var reviewerLLM llm.Provider = reviewerOR

	if zgComputeEnabled() {
		zgModel := envOrDefault("PI_ZG_COMPUTE_MODEL", "qwen/qwen-2.5-7b-instruct")
		zgComp := zg_compute.New(zg_compute.Config{
			BearerToken:      os.Getenv("PI_ZG_COMPUTE_BEARER"),
			ProviderEndpoint: os.Getenv("PI_ZG_COMPUTE_ENDPOINT"),
			DefaultModel:     zgModel,
		})
		plannerLLM = fallback.New(zgComp, plannerOR, func(err error) {
			slog.Warn("planner sealed inference fell back to openrouter", "err", err)
		})
		reviewerLLM = fallback.New(zgComp, reviewerOR, func(err error) {
			slog.Warn("reviewer sealed inference fell back to openrouter", "err", err)
		})
		slog.Info("0G Compute sealed inference wired", "model", zgModel)
	}

	sw := swarm.New(swarm.Config{
		PlannerLLM:  plannerLLM,
		ReviewerLLM: reviewerLLM,
		Memory:      memProv,
	})
	q.SetSwarm(sw)
	q.SetUserID(strconv.FormatInt(cfg.TelegramAllowedUserID, 10))

	if zgINFTEnabled() {
		inftProv, err := zg_7857.New(zg_7857.Config{
			ContractAddress: os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS"),
			EVMRPCURL:       os.Getenv("PI_ZG_EVM_RPC"),
			PrivateKey:      os.Getenv("PI_ZG_PRIVATE_KEY"),
			ChainID:         16602, // 0G Galileo testnet
		})
		if err != nil {
			fail(fmt.Errorf("zg_7857 provider: %w", err))
		}
		defer inftProv.Close()
		q.SetINFT(inftProv)
		slog.Info("0G iNFT registry wired",
			"contract", os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS"))
	}

	if n, err := q.Reconcile(ctx); err != nil {
		slog.Error("reconcile", "err", err)
	} else if n > 0 {
		slog.Warn("reconciled orphan running tasks", "count", n)
	}

	client, err := telegram.NewClient(cfg.TelegramToken, cfg.TelegramAllowedUserID)
	if err != nil {
		fail(err)
	}
	notifier := &tgNotifier{
		client:      client,
		chatID:      cfg.TelegramAllowedUserID,
		sandboxRepo: cfg.GitHubSandboxRepo,
		repo:        repo,
	}

	if ensEnabled() {
		ensProv, err := ens.New(ens.Config{
			ParentName: os.Getenv("PI_ENS_PARENT_NAME"),
			RPCURL:     os.Getenv("PI_ENS_RPC"),
			PrivateKey: os.Getenv("PI_ZG_PRIVATE_KEY"),
			ChainID:    11155111, // Sepolia
		})
		if err != nil {
			// ENS is decorative; Sepolia public RPC flakes more than 0G's RPC.
			// Log + continue without ENS instead of aborting boot.
			slog.Error("ens disabled — boot continues without ENS", "err", err)
		} else {
			defer ensProv.Close()

			inftAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
			for _, p := range []struct{ label, tokenID, zgURI string }{
				{"planner", "0", plannerZGURI},
				{"coder", "1", coderZGURI},
				{"reviewer", "2", reviewerZGURI},
			} {
				if err := syncPersonaENS(ctx, ensProv, p.label, p.tokenID, inftAddr, p.zgURI); err != nil {
					slog.Warn("ens sync failed", "label", p.label, "err", err)
				}
			}
			notifier.ens = ensProv
			slog.Info("ENS resolver wired", "parent", os.Getenv("PI_ENS_PARENT_NAME"))
		}
	}

	q.SetNotifier(notifier)
	q.SetProgressNotifier(notifier)
	handler := telegram.NewHandler(client, q, repo, cfg.GitHubSandboxRepo)

	updates, err := client.Updates(ctx)
	if err != nil {
		fail(err)
	}

	hour, minute, _ := config.ParseDigestTime(cfg.DigestTimeUTC)
	go runDigestScheduler(ctx, hour, minute, repo, client, cfg.TelegramAllowedUserID)

	// Task-execution loop: poll the queue and run one task per tick.
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ran, err := q.RunNext(ctx)
				if err != nil {
					slog.Error("run next", "err", err)
				}
				if ran {
					slog.Info("task run cycle finished")
				}
			}
		}
	}()

	slog.Info("orchestrator ready",
		"version", version,
		"db_path", cfg.DBPath,
		"sandbox_repo", cfg.GitHubSandboxRepo,
	)

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case u, ok := <-updates:
			if !ok {
				slog.Info("updates channel closed")
				return
			}
			if err := handler.Handle(ctx, u); err != nil {
				slog.Error("handler", "err", err)
			}
		}
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
	os.Exit(1)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ENSResolver is the notifier's view of the ENS provider — only the read +
// parent-name calls. Defined here so tests can inject a stub without pulling
// the full era-brain.identity.Resolver interface (writes are out of scope
// for the DM render path).
type ENSResolver interface {
	ReadTextRecord(ctx context.Context, label, key string) (string, error)
	ParentName() string
}

type tgNotifier struct {
	client       telegram.Client
	chatID       int64
	sandboxRepo  string   // "owner/repo"
	repo         *db.Repo // for SetCompletionMessageID
	progressMsgs sync.Map // taskID (int64) → telegram message ID (int64)
	ens          ENSResolver // may be nil — set by main when ENS is wired
}

func (n *tgNotifier) NotifyCompleted(ctx context.Context, a queue.CompletedArgs) {
	body := fmt.Sprintf("✅ task #%d completed", a.TaskID)
	if a.Repo != "" {
		body += "\nrepo: " + a.Repo
	}
	if a.Branch != "" {
		body += "\nbranch: " + a.Branch
	}
	if a.PRURL != "" {
		body += "\npr: " + a.PRURL
	}
	if a.Summary != "" {
		body += "\n\n" + queue.Truncate(a.Summary, 1500)
	}
	body += fmt.Sprintf("\n\ntokens: %d · cost: $%.4f", a.Tokens, float64(a.CostCents)/100)

	if a.PlannerPlan != "" {
		body += "\n\n— planner: " + queue.Truncate(a.PlannerPlan, 200)
	}
	if a.ReviewerDecision != "" {
		rev := "— reviewer: " + a.ReviewerDecision
		if a.ReviewerCritique != "" {
			rev += " — " + queue.Truncate(a.ReviewerCritique, 200)
		}
		body += "\n" + rev
	}

	body += ensFooter(ctx, n.ens)

	msgID, err := n.client.SendMessage(ctx, n.chatID, body)
	if err != nil {
		slog.Error("notify completed", "err", err, "task", a.TaskID)
		return
	}
	if err := n.repo.SetCompletionMessageID(ctx, a.TaskID, msgID); err != nil {
		slog.Warn("set completion message id", "err", err, "task", a.TaskID)
	}
}

func (n *tgNotifier) NotifyFailed(ctx context.Context, id int64, reason string) {
	msg := fmt.Sprintf("task #%d failed: %s", id, truncateForTelegram(scrubSecrets(reason), 3500))
	_, err := n.client.SendMessage(ctx, n.chatID, msg)
	if err != nil {
		slog.Error("notify failed", "err", err, "task", id)
	}
}

func (n *tgNotifier) NotifyCancelled(ctx context.Context, id int64) {
	msg := fmt.Sprintf("task #%d cancelled mid-run", id)
	_, err := n.client.SendMessage(ctx, n.chatID, msg)
	if err != nil {
		slog.Error("notify cancelled", "err", err, "task", id)
	}
}

func (n *tgNotifier) NotifyProgress(ctx context.Context, id int64, ev queue.ProgressEvent) {
	if ev.Action == "" {
		return
	}
	body := fmt.Sprintf("task #%d · iter %d · %s · $%.3f",
		id, ev.Iter, ev.Action, float64(ev.CostCents)/100.0)
	if existing, ok := n.progressMsgs.Load(id); ok {
		msgID := existing.(int64)
		if err := n.client.EditMessageText(ctx, n.chatID, int(msgID), body); err != nil {
			slog.Warn("edit progress", "err", err, "task", id)
		}
		return
	}
	msgID, err := n.client.SendMessage(ctx, n.chatID, body)
	if err != nil {
		slog.Warn("send progress", "err", err, "task", id)
		return
	}
	n.progressMsgs.Store(id, msgID)
}

func (n *tgNotifier) NotifyNeedsReview(ctx context.Context, a queue.NeedsReviewArgs) {
	body := formatNeedsReviewMessage(a) + ensFooter(ctx, n.ens)

	buttons := [][]telegram.InlineButton{
		{
			{Text: "✓ Approve", CallbackData: fmt.Sprintf("approve:%d", a.TaskID)},
			{Text: "✗ Reject", CallbackData: fmt.Sprintf("reject:%d", a.TaskID)},
		},
	}

	_, err := n.client.SendMessageWithButtons(ctx, n.chatID, body, buttons)
	if err != nil {
		slog.Error("notify needs_review", "err", err, "task", a.TaskID)
	}
}

// telegramMaxChars is the budget we allow for the approval DM body.
// Telegram caps messages at 4096 chars; we leave ~300 chars headroom.
const telegramMaxChars = 3800

// formatNeedsReviewMessage composes the human-readable approval DM.
func formatNeedsReviewMessage(a queue.NeedsReviewArgs) string {
	var b strings.Builder
	fmt.Fprintf(&b, "task #%d — needs review\n", a.TaskID)
	fmt.Fprintf(&b, "branch: %s\n", a.Branch)
	fmt.Fprintf(&b, "tokens: %d  cost: $%.4f\n\n", a.Tokens, float64(a.CostCents)/100.0)
	b.WriteString("findings:\n")
	for _, f := range a.Findings {
		fmt.Fprintf(&b, "  • %s (%s): %s\n", f.Rule, f.Path, truncate(f.Message, 100))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "pr: %s\n\n", a.PRURL)
	b.WriteString("diff preview:\n")
	diffPreview := buildDiffPreview(a.Diffs, telegramMaxChars-b.Len()-200)
	b.WriteString(diffPreview)

	if a.PlannerPlan != "" {
		fmt.Fprintf(&b, "\n— planner: %s", queue.Truncate(a.PlannerPlan, 200))
	}
	if a.ReviewerDecision != "" {
		rev := "— reviewer: " + a.ReviewerDecision
		if a.ReviewerCritique != "" {
			rev += " — " + queue.Truncate(a.ReviewerCritique, 200)
		}
		fmt.Fprintf(&b, "\n%s", rev)
	}

	return b.String()
}

// ensFooter renders the "personas:" section appended to completion / review DMs.
// Returns "" when ens is nil OR any single read fails OR any persona's records
// are missing (partial data → empty string, never partial footer).
func ensFooter(ctx context.Context, ens ENSResolver) string {
	if ens == nil {
		return ""
	}
	type row struct{ label, addr, tokenID string }
	rows := make([]row, 0, 3)
	for _, label := range []string{"planner", "coder", "reviewer"} {
		addr, err := ens.ReadTextRecord(ctx, label, "inft_addr")
		if err != nil {
			return ""
		}
		tokenID, err := ens.ReadTextRecord(ctx, label, "inft_token_id")
		if err != nil {
			return ""
		}
		if addr == "" || tokenID == "" {
			return ""
		}
		rows = append(rows, row{label: label, addr: addr, tokenID: tokenID})
	}
	var b strings.Builder
	b.WriteString("\n\npersonas:")
	parent := ens.ParentName()
	for _, r := range rows {
		shortAddr := r.addr
		if len(shortAddr) > 12 {
			shortAddr = shortAddr[:12] + "…"
		}
		fmt.Fprintf(&b, "\n  %s.%s → token #%s (%s)", r.label, parent, r.tokenID, shortAddr)
	}
	return b.String()
}

// buildDiffPreview renders up to budget chars of unified-diff-style preview
// across all files in order.
func buildDiffPreview(files []diffscan.FileDiff, budget int) string {
	if budget <= 0 {
		return "(diff too large to preview; open the compare link)"
	}
	var b strings.Builder
	for _, f := range files {
		if b.Len() > budget {
			b.WriteString("\n…(truncated)")
			return b.String()
		}
		deletedSuffix := map[bool]string{true: " (deleted)", false: ""}[f.Deleted]
		fmt.Fprintf(&b, "--- %s%s\n", f.Path, deletedSuffix)
		for _, line := range f.Removed {
			if b.Len() > budget {
				b.WriteString("\n…(truncated)")
				return b.String()
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
		for _, line := range f.Added {
			if b.Len() > budget {
				b.WriteString("\n…(truncated)")
				return b.String()
			}
			fmt.Fprintf(&b, "+ %s\n", line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// truncateForTelegram caps s at `budget` bytes, appending a rune-safe footer
// if anything was dropped. Backs up past any partial multi-byte rune so the
// returned prefix is always valid UTF-8.
func truncateForTelegram(s string, budget int) string {
	if len(s) <= budget {
		return s
	}
	cut := budget
	// If s[cut] is a continuation byte (0b10xxxxxx), we're mid-rune.
	// Back up until cut sits at a rune-start boundary.
	for cut > 0 && cut < len(s) && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n…(%d bytes truncated)", len(s)-cut)
}

// compile-time assertions that tgNotifier satisfies both notifier interfaces
var _ queue.Notifier = (*tgNotifier)(nil)
var _ queue.ProgressNotifier = (*tgNotifier)(nil)

// zgEnabled returns true when all required 0G testnet env vars are present.
// PI_ZG_KV_NODE is optional — its absence just means reads return ErrNotFound,
// which dual.Provider correctly falls through to the SQLite cache.
func zgEnabled() bool {
	return os.Getenv("PI_ZG_PRIVATE_KEY") != "" &&
		os.Getenv("PI_ZG_EVM_RPC") != "" &&
		os.Getenv("PI_ZG_INDEXER_RPC") != ""
}

// zgComputeEnabled returns true when all required 0G Compute env vars are present.
// PI_ZG_COMPUTE_MODEL is optional (defaults to qwen/qwen-2.5-7b-instruct).
func zgComputeEnabled() bool {
	return os.Getenv("PI_ZG_COMPUTE_ENDPOINT") != "" &&
		os.Getenv("PI_ZG_COMPUTE_BEARER") != ""
}

// zgINFTEnabled returns true when the iNFT contract address is configured
// AND a private key is available for tx signing.
func zgINFTEnabled() bool {
	return os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS") != "" &&
		os.Getenv("PI_ZG_PRIVATE_KEY") != ""
}

// ensEnabled returns true when the ENS parent name + Sepolia RPC are configured
// AND a private key is available for tx signing.
func ensEnabled() bool {
	return os.Getenv("PI_ENS_RPC") != "" &&
		os.Getenv("PI_ENS_PARENT_NAME") != "" &&
		os.Getenv("PI_ZG_PRIVATE_KEY") != ""
}

// syncPersonaENS registers the subname and writes 3 text records for a single
// persona. Each step is independently idempotent — re-running this on a fully
// synced subname produces 0 on-chain txs (just 4 RPC reads).
func syncPersonaENS(ctx context.Context, p *ens.Provider, label, tokenID, inftAddr, zgURI string) error {
	if err := p.EnsureSubname(ctx, label); err != nil {
		return fmt.Errorf("ensureSubname: %w", err)
	}
	if err := p.SetTextRecord(ctx, label, "inft_addr", inftAddr); err != nil {
		return fmt.Errorf("set inft_addr: %w", err)
	}
	if err := p.SetTextRecord(ctx, label, "inft_token_id", tokenID); err != nil {
		return fmt.Errorf("set inft_token_id: %w", err)
	}
	if err := p.SetTextRecord(ctx, label, "zg_storage_uri", zgURI); err != nil {
		return fmt.Errorf("set zg_storage_uri: %w", err)
	}
	return nil
}

// zgComposite combines zg_kv (KV ops) and zg_log (Log ops) into a single
// memory.Provider, used as the Primary in the dual provider. Both sub-providers
// share the same underlying *zg_kv.LiveOps so we open the SDK clients once.
type zgComposite struct {
	kvP  memory.Provider
	logP memory.Provider
}

func (c *zgComposite) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
	return c.kvP.GetKV(ctx, ns, key)
}
func (c *zgComposite) PutKV(ctx context.Context, ns, key string, val []byte) error {
	return c.kvP.PutKV(ctx, ns, key, val)
}
func (c *zgComposite) AppendLog(ctx context.Context, ns string, entry []byte) error {
	return c.logP.AppendLog(ctx, ns, entry)
}
func (c *zgComposite) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
	return c.logP.ReadLog(ctx, ns)
}

// runDigestScheduler fires once per day at hour:minute UTC and sends a
// digest message to chatID. Respects ctx for graceful shutdown.
func runDigestScheduler(ctx context.Context, hour, minute int, repo *db.Repo, client telegram.Client, chatID int64) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		wait := time.Until(next)
		slog.Info("digest scheduled", "fires_at_utc", next.Format(time.RFC3339), "in", wait.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		// Fire: fetch last 24h, render, send.
		to := time.Now().UTC()
		from := to.Add(-24 * time.Hour)
		tasks, err := repo.ListBetween(ctx, from, to)
		if err != nil {
			slog.Error("digest listbetween", "err", err)
			continue
		}
		msg := digest.Render(tasks, from, to)
		_, err = client.SendMessage(ctx, chatID, msg)
		if err != nil {
			slog.Error("digest send", "err", err)
			continue
		}
		slog.Info("digest sent", "tasks", len(tasks))
	}
}
