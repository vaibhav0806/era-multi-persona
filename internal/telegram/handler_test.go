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
	Created []string
	Status  map[int64]string
	Listed  bool

	// M3 additions:
	LastApprovalData string
	ApprovalReply    string
	ApprovalErr      error
	CancelledIDs     []int64
	RetriedIDs       []int64
	RetryNewID       int64
	RetryErr         error
}

func (s *stubOps) CreateTask(ctx context.Context, desc string) (int64, error) {
	s.Created = append(s.Created, desc)
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
	return nil
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
		ChatID: 1,
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
		ChatID: 1,
		Callback: &CallbackQuery{ID: "cb2", MessageID: 5, Data: "reject:99"},
	})
	// Handler should not return an error — it answered the callback with the error text.
	require.NoError(t, err)
	require.Len(t, fc.AnsweredCallbacks, 1)
	require.Contains(t, fc.AnsweredCallbacks[0].Text, "not found")
}
