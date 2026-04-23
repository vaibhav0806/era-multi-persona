package telegram

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// FakeClient is exported so handler_test.go (Task 9) can reuse it.
type FakeClient struct {
	mu sync.Mutex

	// text messages
	Sent []struct {
		ChatID int64
		Text   string
	}

	// button messages
	SentWithButtons []struct {
		ChatID  int64
		Text    string
		Buttons [][]InlineButton
	}
	nextMessageID int

	EditedMessages []struct {
		ChatID    int64
		MessageID int
		Text      string
	}

	AnsweredCallbacks []struct {
		ID   string
		Text string
	}

	// inbound
	Incoming          chan Update
	IncomingCallbacks chan CallbackQuery
}

// compile-time check that FakeClient satisfies Client.
var _ Client = (*FakeClient)(nil)

func NewFakeClient() *FakeClient {
	return &FakeClient{
		Incoming:          make(chan Update, 16),
		IncomingCallbacks: make(chan CallbackQuery, 16),
	}
}

func (f *FakeClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Sent = append(f.Sent, struct {
		ChatID int64
		Text   string
	}{chatID, text})
	return nil
}

func (f *FakeClient) SendMessageWithButtons(ctx context.Context, chatID int64, text string, buttons [][]InlineButton) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextMessageID++
	f.SentWithButtons = append(f.SentWithButtons, struct {
		ChatID  int64
		Text    string
		Buttons [][]InlineButton
	}{chatID, text, buttons})
	return f.nextMessageID, nil
}

func (f *FakeClient) EditMessageText(ctx context.Context, chatID int64, messageID int, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.EditedMessages = append(f.EditedMessages, struct {
		ChatID    int64
		MessageID int
		Text      string
	}{chatID, messageID, text})
	return nil
}

func (f *FakeClient) AnswerCallback(ctx context.Context, callbackID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AnsweredCallbacks = append(f.AnsweredCallbacks, struct {
		ID   string
		Text string
	}{callbackID, text})
	return nil
}

func (f *FakeClient) Updates(ctx context.Context) (<-chan Update, error) {
	out := make(chan Update)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case u, ok := <-f.Incoming:
				if !ok {
					return
				}
				out <- u
			case cb, ok := <-f.IncomingCallbacks:
				if !ok {
					continue
				}
				out <- Update{Callback: &cb}
			}
		}
	}()
	return out, nil
}

func TestFakeClient_SendMessageWithButtons(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := NewFakeClient()
	buttons := [][]InlineButton{
		{{Text: "Approve", CallbackData: "approve:1"}, {Text: "Reject", CallbackData: "reject:1"}},
	}
	id, err := f.SendMessageWithButtons(ctx, 1, "needs review", buttons)
	require.NoError(t, err)
	require.NotZero(t, id)

	require.Len(t, f.SentWithButtons, 1)
	require.Equal(t, "needs review", f.SentWithButtons[0].Text)
	require.Equal(t, buttons, f.SentWithButtons[0].Buttons)
}

func TestFakeClient_CallbackQueryPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := NewFakeClient()
	updates, err := f.Updates(ctx)
	require.NoError(t, err)

	f.IncomingCallbacks <- CallbackQuery{ID: "cb1", MessageID: 99, Data: "approve:42"}
	got := <-updates
	require.NotNil(t, got.Callback)
	require.Equal(t, "cb1", got.Callback.ID)
	require.Equal(t, 99, got.Callback.MessageID)
	require.Equal(t, "approve:42", got.Callback.Data)
	require.Empty(t, got.Text, "callback update has no text")
}

func TestFakeClient_AnswerCallbackRecorded(t *testing.T) {
	ctx := context.Background()
	f := NewFakeClient()
	require.NoError(t, f.AnswerCallback(ctx, "cb1", "task #42 approved"))
	require.Len(t, f.AnsweredCallbacks, 1)
	require.Equal(t, "cb1", f.AnsweredCallbacks[0].ID)
	require.Equal(t, "task #42 approved", f.AnsweredCallbacks[0].Text)
}

func TestFakeClient_EditMessageText(t *testing.T) {
	ctx := context.Background()
	f := NewFakeClient()
	require.NoError(t, f.EditMessageText(ctx, 5, 100, "approved ✓"))
	require.Len(t, f.EditedMessages, 1)
	require.Equal(t, int64(5), f.EditedMessages[0].ChatID)
	require.Equal(t, 100, f.EditedMessages[0].MessageID)
	require.Equal(t, "approved ✓", f.EditedMessages[0].Text)
}

func TestFakeClient_RoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := NewFakeClient()
	updates, err := f.Updates(ctx)
	require.NoError(t, err)

	f.Incoming <- Update{UserID: 1, ChatID: 1, Text: "hi"}
	got := <-updates
	require.Equal(t, "hi", got.Text)

	require.NoError(t, f.SendMessage(ctx, 1, "hello"))
	require.Len(t, f.Sent, 1)
	require.Equal(t, "hello", f.Sent[0].Text)
}
