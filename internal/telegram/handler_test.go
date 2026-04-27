package telegram

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/persona"
	"github.com/vaibhav0806/era/internal/stats"
)

// stubOps records calls instead of touching a real DB.
type stubOps struct {
	Created         []string
	LastCreatedRepo string
	LastProfile     string
	Status          map[int64]string
	Listed          bool

	// M3 additions:
	LastApprovalData string
	ApprovalReply    string
	ApprovalErr      error
	CancelledIDs     []int64
	CancelErr        error
	RetriedIDs       []int64
	RetryNewID       int64
	RetryErr         error

	// AI-6: reply routing
	nextID   int64
	lastRepo string
	lastDesc string

	// M7-F.4: persona flag plumbing
	lastPersonaName string

	// AK-3: ask routing
	lastAskRepo string
	lastAskDesc string

	// AL-3: stats
	statsResult stats.Stats

	// M7-F.3: persona ops
	mintCalled bool
	mintName   string
	mintPrompt string
	mintErr    error
	mintResult PersonaMintResult
	listResult []persona.Persona
	listErr    error
}

func (s *stubOps) CreateTask(ctx context.Context, desc, targetRepo, profile, personaName string) (int64, error) {
	s.Created = append(s.Created, desc)
	s.LastCreatedRepo = targetRepo
	s.LastProfile = profile
	s.lastRepo = targetRepo
	s.lastDesc = desc
	s.lastPersonaName = personaName
	if s.nextID != 0 {
		return s.nextID, nil
	}
	return int64(len(s.Created)), nil
}
func (s *stubOps) TaskStatus(ctx context.Context, id int64) (string, error) {
	if v, ok := s.Status[id]; ok {
		return v, nil
	}
	return "", ErrTaskNotFound
}
func (s *stubOps) ListRecent(ctx context.Context, limit int) ([]TaskSummary, error) {
	s.Listed = true
	return []TaskSummary{{ID: 1, Description: "t1", Status: "queued"}}, nil
}

func (s *stubOps) HandleApproval(ctx context.Context, data string) (string, error) {
	s.LastApprovalData = data
	return s.ApprovalReply, s.ApprovalErr
}

// Stubs for M3-9 (CancelTask, RetryTask). They exist here because the Ops
// interface includes them; compile-time assertion needs them now.
func (s *stubOps) CancelTask(ctx context.Context, id int64) error {
	s.CancelledIDs = append(s.CancelledIDs, id)
	return s.CancelErr
}
func (s *stubOps) RetryTask(ctx context.Context, id int64) (int64, error) {
	s.RetriedIDs = append(s.RetriedIDs, id)
	if s.RetryErr != nil {
		return 0, s.RetryErr
	}
	if s.RetryNewID != 0 {
		return s.RetryNewID, nil
	}
	return id + 100, nil
}

func (s *stubOps) CreateAskTask(ctx context.Context, desc, targetRepo string) (int64, error) {
	s.lastAskDesc = desc
	s.lastAskRepo = targetRepo
	return s.nextID, nil
}

func (s *stubOps) Stats(ctx context.Context) (stats.Stats, error) {
	return s.statsResult, nil
}

func (s *stubOps) MintPersona(ctx context.Context, name, prompt string) (PersonaMintResult, error) {
	s.mintCalled = true
	s.mintName = name
	s.mintPrompt = prompt
	return s.mintResult, s.mintErr
}

func (s *stubOps) ListPersonas(ctx context.Context) ([]persona.Persona, error) {
	return s.listResult, s.listErr
}

// compile-time assertion that stubOps satisfies Ops
var _ Ops = (*stubOps)(nil)

func TestHandler_TaskCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	err := h.Handle(context.Background(), Update{ChatID: 42, Text: "/task build auth flow"})
	require.NoError(t, err)
	require.Equal(t, []string{"build auth flow"}, ops.Created)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "queued")
}

func TestHandler_StatusCommand(t *testing.T) {
	ops := &stubOps{Status: map[int64]string{7: "running"}}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/status 7"}))
	require.Contains(t, strings.ToLower(fc.Sent[0].Text), "running")
}

func TestHandler_StatusUnknownTask(t *testing.T) {
	ops := &stubOps{Status: map[int64]string{}}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/status 99"}))
	require.Contains(t, fc.Sent[0].Text, "not found")
}

func TestHandler_ListCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/list"}))
	require.True(t, ops.Listed)
	require.Contains(t, fc.Sent[0].Text, "t1")
}

func TestHandler_UnknownCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/wat"}))
	require.Contains(t, strings.ToLower(fc.Sent[0].Text), "unknown")
}

func TestHandler_CallbackQueryDispatched(t *testing.T) {
	ops := &stubOps{ApprovalReply: "task #42 approved"}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	err := h.Handle(context.Background(), Update{
		ChatID:   1,
		Callback: &CallbackQuery{ID: "cb1", MessageID: 99, Data: "approve:42"},
	})
	require.NoError(t, err)
	require.Equal(t, "approve:42", ops.LastApprovalData)

	// Handler should have called AnswerCallback with the reply
	require.Len(t, fc.AnsweredCallbacks, 1)
	require.Equal(t, "cb1", fc.AnsweredCallbacks[0].ID)
	require.Equal(t, "task #42 approved", fc.AnsweredCallbacks[0].Text)
}

func TestHandler_CallbackQuery_OpsError(t *testing.T) {
	ops := &stubOps{ApprovalErr: errors.New("not found")}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	err := h.Handle(context.Background(), Update{
		ChatID:   1,
		Callback: &CallbackQuery{ID: "cb2", MessageID: 5, Data: "reject:99"},
	})
	// Handler should not return an error — it answered the callback with the error text.
	require.NoError(t, err)
	require.Len(t, fc.AnsweredCallbacks, 1)
	require.Contains(t, fc.AnsweredCallbacks[0].Text, "not found")
}

func TestHandler_CancelCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/cancel 42"}))
	require.Equal(t, []int64{42}, ops.CancelledIDs)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "cancel")
}

func TestHandler_CancelBadArg(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/cancel abc"}))
	require.Empty(t, ops.CancelledIDs)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "usage:")
}

func TestHandler_RetryCommand(t *testing.T) {
	ops := &stubOps{RetryNewID: 150}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/retry 42"}))
	require.Equal(t, []int64{42}, ops.RetriedIDs)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "#150")
}

func TestHandler_RetryError(t *testing.T) {
	ops := &stubOps{RetryErr: errors.New("task not found")}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/retry 42"}))
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "not found")
}

func TestHandler_CancelError(t *testing.T) {
	ops := &stubOps{CancelErr: errors.New("running tasks cannot be cancelled")}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/cancel 42"}))
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "cannot be cancelled")
}

func TestParseTaskArgs(t *testing.T) {
	tests := []struct {
		in       string
		wantRepo string
		wantDesc string
	}{
		{"add a file", "", "add a file"},
		{"vaibhav0806/era add a file", "vaibhav0806/era", "add a file"},
		{"vaibhav0806/era", "vaibhav0806/era", ""},
		{"alice/foo-bar refactor stuff", "alice/foo-bar", "refactor stuff"},
		{"justwords no slash", "", "justwords no slash"},
		{"x/y.z multiple words", "x/y.z", "multiple words"},
		{"", "", ""},
	}
	for _, tc := range tests {
		gotRepo, gotDesc := parseTaskArgs(tc.in)
		require.Equal(t, tc.wantRepo, gotRepo, "input: %q", tc.in)
		require.Equal(t, tc.wantDesc, gotDesc, "input: %q", tc.in)
	}
}

func TestHandler_TaskCommand_WithRepo(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/task vaibhav0806/foo build auth"}))
	require.Equal(t, []string{"build auth"}, ops.Created)
	require.Equal(t, "vaibhav0806/foo", ops.LastCreatedRepo)
	require.Contains(t, fc.Sent[0].Text, "vaibhav0806/foo")
}

func TestHandler_TaskCommand_WithoutRepo_UsesDefault(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/task add a file"}))
	require.Equal(t, []string{"add a file"}, ops.Created)
	require.Equal(t, "", ops.LastCreatedRepo)
	require.NotContains(t, fc.Sent[0].Text, "repo:")
}

func newInMemRepo(t *testing.T) *db.Repo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return db.NewRepo(h)
}

func TestHandler_ReplyToUnknownMessage_DMsNotFound(t *testing.T) {
	ctx := context.Background()
	f := NewFakeClient()
	h := NewHandler(f, &stubOps{}, newInMemRepo(t), "vaibhav0806/sandbox")
	err := h.Handle(ctx, Update{
		ChatID:           1,
		Text:             "now add tests",
		ReplyToMessageID: 99999, // nothing in DB matches
	})
	require.NoError(t, err)
	require.Len(t, f.Sent, 1)
	require.Contains(t, f.Sent[0].Text, "couldn't find")
}

func TestHandler_ReplyToKnownMessage_QueuesThreadedTask(t *testing.T) {
	ctx := context.Background()
	repo := newInMemRepo(t)
	task, err := repo.CreateTask(ctx, "build a thing", "vaibhav0806/foo", "default", "")
	require.NoError(t, err)
	require.NoError(t, repo.SetCompletionMessageID(ctx, task.ID, 12345))

	f := NewFakeClient()
	ops := &stubOps{nextID: 99}
	h := NewHandler(f, ops, repo, "vaibhav0806/sandbox")
	err = h.Handle(ctx, Update{
		ChatID:           1,
		Text:             "now add tests",
		ReplyToMessageID: 12345,
	})
	require.NoError(t, err)
	require.Equal(t, "vaibhav0806/foo", ops.lastRepo)
	require.Contains(t, ops.lastDesc, "previously completed task #")
	require.Contains(t, ops.lastDesc, "now add tests")
	require.Contains(t, f.Sent[0].Text, "task #99 queued")
	require.Contains(t, f.Sent[0].Text, "reply to #")
}

func TestHandler_AskCommand_QueuesReadOnlyTask(t *testing.T) {
	ctx := context.Background()
	f := NewFakeClient()
	ops := &stubOps{nextID: 42}
	h := NewHandler(f, ops, nil, "vaibhav0806/sandbox")
	err := h.Handle(ctx, Update{
		ChatID: 1,
		Text:   "/ask vaibhav0806/foo what is in main.go",
	})
	require.NoError(t, err)
	require.Equal(t, "vaibhav0806/foo", ops.lastAskRepo)
	require.Equal(t, "what is in main.go", ops.lastAskDesc)
	require.Contains(t, f.Sent[0].Text, "task #42 queued (ask")
}

func TestHandler_AskWithoutRepo_DMsUsage(t *testing.T) {
	ctx := context.Background()
	f := NewFakeClient()
	h := NewHandler(f, &stubOps{}, nil, "vaibhav0806/sandbox")
	err := h.Handle(ctx, Update{ChatID: 1, Text: "/ask just a question"})
	require.NoError(t, err)
	require.Contains(t, f.Sent[0].Text, "usage: /ask")
}

func TestHandler_ReplyWithCommandPrefix_FallsThroughToCommand(t *testing.T) {
	ctx := context.Background()
	f := NewFakeClient()
	h := NewHandler(f, &stubOps{}, newInMemRepo(t), "vaibhav0806/sandbox")
	err := h.Handle(ctx, Update{
		ChatID:           1,
		Text:             "/list",
		ReplyToMessageID: 12345,
	})
	require.NoError(t, err)
	require.NotContains(t, f.Sent[0].Text, "couldn't find")
}

func TestHandler_StatsCommand_SendsFormattedDM(t *testing.T) {
	ctx := context.Background()
	f := NewFakeClient()
	ops := &stubOps{
		statsResult: stats.Stats{
			Last24h:      stats.PeriodStats{TasksTotal: 5, TasksOK: 4, Tokens: 1500, CostCents: 8},
			Last7d:       stats.PeriodStats{TasksTotal: 20, TasksOK: 17, Tokens: 8500, CostCents: 75},
			Last30d:      stats.PeriodStats{TasksTotal: 80, TasksOK: 65, Tokens: 41000, CostCents: 320},
			PendingQueue: 0,
		},
	}
	h := NewHandler(f, ops, nil, "vaibhav0806/sandbox")
	err := h.Handle(ctx, Update{ChatID: 1, Text: "/stats"})
	require.NoError(t, err)
	require.Len(t, f.Sent, 1)
	body := f.Sent[0].Text
	require.Contains(t, body, "era stats")
	require.Contains(t, body, "tasks:")
	require.Contains(t, body, "5")
	require.Contains(t, body, "80")
	require.Contains(t, body, "queue: 0 pending")
}

func TestHandle_PersonaMint_Success(t *testing.T) {
	ops := &stubOps{
		mintResult: PersonaMintResult{
			TokenID:         "3",
			MintTxHash:      "0xabc",
			ENSSubname:      "rustacean.vaibhav-era.eth",
			SystemPromptURI: "zg://hash",
		},
	}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1,
		Text:   "/persona-mint rustacean You only write idiomatic Rust code. Never compromise on memory safety or borrow-checker correctness.",
	}))

	require.True(t, ops.mintCalled)
	require.Equal(t, "rustacean", ops.mintName)
	require.Contains(t, ops.mintPrompt, "idiomatic Rust")
	require.NotEmpty(t, fc.Sent)
	last := fc.Sent[len(fc.Sent)-1].Text
	require.Contains(t, last, "token #3")
	require.Contains(t, last, "rustacean.vaibhav-era.eth")
	require.Contains(t, last, "0xabc")
	require.Contains(t, last, "zg://hash")
}

func TestHandle_PersonaMint_InvalidName_NoChainCalls(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"uppercase", "/persona-mint RustLover prompt text long enough to satisfy minimum length"},
		{"too_short", "/persona-mint xy prompt text long enough to satisfy minimum length"},
		{"reserved_planner", "/persona-mint planner prompt text long enough to satisfy minimum length"},
		{"reserved_coder", "/persona-mint coder prompt text long enough to satisfy minimum length"},
		{"reserved_reviewer", "/persona-mint reviewer prompt text long enough to satisfy minimum length"},
		{"empty_prompt", "/persona-mint rustacean"},
		{"prompt_too_short", "/persona-mint rustacean tiny"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ops := &stubOps{}
			fc := NewFakeClient()
			h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
			require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: c.text}))
			require.False(t, ops.mintCalled, "should not call MintPersona on invalid input")
			require.NotEmpty(t, fc.Sent)
		})
	}
}

func TestHandle_PersonaMint_DuplicateName(t *testing.T) {
	ops := &stubOps{mintErr: persona.ErrPersonaNameTaken}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1,
		Text:   "/persona-mint rustacean You only write idiomatic Rust code, no exceptions whatsoever.",
	}))
	require.NotEmpty(t, fc.Sent)
	last := fc.Sent[len(fc.Sent)-1].Text
	require.Contains(t, last, "already taken")
}

func TestHandle_Task_WithPersonaFlag(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1,
		Text:   "/task --persona=rustacean fix the auth bug",
	}))
	require.Equal(t, "rustacean", ops.lastPersonaName)
	require.Equal(t, "fix the auth bug", ops.lastDesc)
}

func TestHandle_Task_NoPersonaFlag_DefaultsToEmpty(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1,
		Text:   "/task fix the auth bug",
	}))
	require.Equal(t, "", ops.lastPersonaName)
	require.Equal(t, "fix the auth bug", ops.lastDesc)
}

func TestHandle_Task_PersonaFlagBeforeRepo(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")

	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1,
		Text:   "/task --persona=rustacean foo/bar fix the auth bug",
	}))
	require.Equal(t, "rustacean", ops.lastPersonaName)
	require.Equal(t, "foo/bar", ops.lastRepo)
	require.Equal(t, "fix the auth bug", ops.lastDesc)
}

func TestHandle_Personas_Lists(t *testing.T) {
	ops := &stubOps{
		listResult: []persona.Persona{
			{TokenID: "0", Name: "planner", ENSSubname: "planner.vaibhav-era.eth", Description: "default planner"},
			{TokenID: "3", Name: "rustacean", ENSSubname: "rustacean.vaibhav-era.eth", Description: "Rust-only persona"},
		},
	}
	fc := NewFakeClient()
	h := NewHandler(fc, ops, nil, "vaibhav0806/sandbox")
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/personas"}))
	require.NotEmpty(t, fc.Sent)
	body := fc.Sent[len(fc.Sent)-1].Text
	require.Contains(t, body, "planner.vaibhav-era.eth")
	require.Contains(t, body, "rustacean.vaibhav-era.eth")
	require.Contains(t, body, "#0")
	require.Contains(t, body, "#3")
}
