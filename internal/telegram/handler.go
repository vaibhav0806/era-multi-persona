package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrTaskNotFound is returned by Ops.TaskStatus when the task ID is unknown.
var ErrTaskNotFound = errors.New("task not found")

type TaskSummary struct {
	ID          int64
	Description string
	Status      string
	BranchName  string
}

// Ops is the subset of orchestrator functionality the handler needs. Kept
// narrow so we can stub it in tests. Implemented by internal/queue.Queue in
// Task 10.
type Ops interface {
	CreateTask(ctx context.Context, desc string) (int64, error)
	TaskStatus(ctx context.Context, id int64) (string, error)
	ListRecent(ctx context.Context, limit int) ([]TaskSummary, error)
	HandleApproval(ctx context.Context, data string) (replyText string, err error)
	// M3-9 command wires: the interface methods are declared here;
	// the handler dispatch lines are added in M3-9.
	CancelTask(ctx context.Context, id int64) error
	RetryTask(ctx context.Context, id int64) (newID int64, err error)
}

type Handler struct {
	client Client
	ops    Ops
}

func NewHandler(c Client, ops Ops) *Handler { return &Handler{client: c, ops: ops} }

func (h *Handler) Handle(ctx context.Context, u Update) error {
	// M3: callback queries (button taps)
	if u.Callback != nil {
		return h.handleCallback(ctx, u)
	}

	text := strings.TrimSpace(u.Text)
	switch {
	case strings.HasPrefix(text, "/task "):
		desc := strings.TrimSpace(strings.TrimPrefix(text, "/task "))
		if desc == "" {
			return h.client.SendMessage(ctx, u.ChatID, "usage: /task <description>")
		}
		id, err := h.ops.CreateTask(ctx, desc)
		if err != nil {
			return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
		}
		return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d queued", id))

	case strings.HasPrefix(text, "/status "):
		raw := strings.TrimSpace(strings.TrimPrefix(text, "/status "))
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return h.client.SendMessage(ctx, u.ChatID, "usage: /status <task-id>")
		}
		status, err := h.ops.TaskStatus(ctx, id)
		if errors.Is(err, ErrTaskNotFound) {
			return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d not found", id))
		}
		if err != nil {
			return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
		}
		return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d: %s", id, status))

	case text == "/list":
		items, err := h.ops.ListRecent(ctx, 10)
		if err != nil {
			return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
		}
		var b strings.Builder
		if len(items) == 0 {
			b.WriteString("no tasks yet")
		}
		for _, it := range items {
			fmt.Fprintf(&b, "#%d [%s] %s\n", it.ID, it.Status, it.Description)
		}
		return h.client.SendMessage(ctx, u.ChatID, b.String())

	default:
		return h.client.SendMessage(ctx, u.ChatID, "unknown command. try /task, /status, /list")
	}
}

func (h *Handler) handleCallback(ctx context.Context, u Update) error {
	reply, err := h.ops.HandleApproval(ctx, u.Callback.Data)
	if err != nil {
		// Answer with error message so user sees it as a toast; do not
		// bubble up (Telegram callback errors shouldn't crash the loop).
		_ = h.client.AnswerCallback(ctx, u.Callback.ID, "error: "+err.Error())
		return nil
	}
	return h.client.AnswerCallback(ctx, u.Callback.ID, reply)
}
