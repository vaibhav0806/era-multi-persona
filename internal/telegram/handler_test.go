package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
}

func (s *stubOps) CreateTask(ctx context.Context, desc, targetRepo, profile string) (int64, error) {
	s.Created = append(s.Created, desc)
	s.LastCreatedRepo = targetRepo
	s.LastProfile = profile
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

// compile-time assertion that stubOps satisfies Ops
var _ Ops = (*stubOps)(nil)

func TestHandler_TaskCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)

	err := h.Handle(context.Background(), Update{ChatID: 42, Text: "/task build auth flow"})
	require.NoError(t, err)
	require.Equal(t, []string{"build auth flow"}, ops.Created)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "queued")
}

func TestHandler_StatusCommand(t *testing.T) {
	ops := &stubOps{Status: map[int64]string{7: "running"}}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/status 7"}))
	require.Contains(t, strings.ToLower(fc.Sent[0].Text), "running")
}

func TestHandler_StatusUnknownTask(t *testing.T) {
	ops := &stubOps{Status: map[int64]string{}}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/status 99"}))
	require.Contains(t, fc.Sent[0].Text, "not found")
}

func TestHandler_ListCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/list"}))
	require.True(t, ops.Listed)
	require.Contains(t, fc.Sent[0].Text, "t1")
}

func TestHandler_UnknownCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/wat"}))
	require.Contains(t, strings.ToLower(fc.Sent[0].Text), "unknown")
}

func TestHandler_CallbackQueryDispatched(t *testing.T) {
	ops := &stubOps{ApprovalReply: "task #42 approved"}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)

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
	h := NewHandler(fc, ops)

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
	h := NewHandler(fc, ops)

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/cancel 42"}))
	require.Equal(t, []int64{42}, ops.CancelledIDs)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "cancel")
}

func TestHandler_CancelBadArg(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/cancel abc"}))
	require.Empty(t, ops.CancelledIDs)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "usage:")
}

func TestHandler_RetryCommand(t *testing.T) {
	ops := &stubOps{RetryNewID: 150}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/retry 42"}))
	require.Equal(t, []int64{42}, ops.RetriedIDs)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "#150")
}

func TestHandler_RetryError(t *testing.T) {
	ops := &stubOps{RetryErr: errors.New("task not found")}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/retry 42"}))
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "not found")
}

func TestHandler_CancelError(t *testing.T) {
	ops := &stubOps{CancelErr: errors.New("running tasks cannot be cancelled")}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
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
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/task vaibhav0806/foo build auth"}))
	require.Equal(t, []string{"build auth"}, ops.Created)
	require.Equal(t, "vaibhav0806/foo", ops.LastCreatedRepo)
	require.Contains(t, fc.Sent[0].Text, "vaibhav0806/foo")
}

func TestHandler_TaskCommand_WithoutRepo_UsesDefault(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/task add a file"}))
	require.Equal(t, []string{"add a file"}, ops.Created)
	require.Equal(t, "", ops.LastCreatedRepo)
	require.NotContains(t, fc.Sent[0].Text, "repo:")
}
