package telegram

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// InlineButton describes a single tap-target under a Telegram message.
// CallbackData is sent back to the bot when the user taps; keep it small
// (Telegram caps at 64 bytes) and encode it like "action:id" (e.g. "approve:42").
type InlineButton struct {
	Text         string
	CallbackData string
}

// CallbackQuery is an inbound button-tap event.
type CallbackQuery struct {
	ID        string // Telegram callback query id, needed for AnswerCallback
	MessageID int    // message that contained the button
	Data      string // the tapped button's CallbackData
}

// Update is our own domain type, deliberately insulating handlers from the
// tgbotapi library so we can swap libraries or fake it in tests without
// touching handler code.
type Update struct {
	UserID   int64
	ChatID   int64
	Text     string          // set for text messages; empty for callback updates
	Callback *CallbackQuery  // set for button-tap updates; nil for text
}

type Client interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	SendMessageWithButtons(ctx context.Context, chatID int64, text string, buttons [][]InlineButton) (messageID int, err error)
	EditMessageText(ctx context.Context, chatID int64, messageID int, text string) error
	AnswerCallback(ctx context.Context, callbackID string, text string) error
	Updates(ctx context.Context) (<-chan Update, error)
}

type realClient struct {
	api           *tgbotapi.BotAPI
	allowedUserID int64
}

// NewClient connects to the Telegram Bot API using the given token, and
// returns a Client that silently drops messages from any user whose ID
// does not match allowedUserID.
func NewClient(token string, allowedUserID int64) (Client, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot api: %w", err)
	}
	return &realClient{api: api, allowedUserID: allowedUserID}, nil
}

func (c *realClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	// Plain text is safer than MarkdownV2 for arbitrary content; MarkdownV2
	// requires escaping many characters and we'd rather not fight it for M0.
	if _, err := c.api.Send(msg); err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	return nil
}

func (c *realClient) SendMessageWithButtons(ctx context.Context, chatID int64, text string, buttons [][]InlineButton) (int, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(buttons))
	for _, row := range buttons {
		cols := make([]tgbotapi.InlineKeyboardButton, 0, len(row))
		for _, b := range row {
			cols = append(cols, tgbotapi.NewInlineKeyboardButtonData(b.Text, b.CallbackData))
		}
		rows = append(rows, cols)
	}
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	sent, err := c.api.Send(msg)
	if err != nil {
		return 0, fmt.Errorf("telegram send with buttons: %w", err)
	}
	return sent.MessageID, nil
}

func (c *realClient) EditMessageText(ctx context.Context, chatID int64, messageID int, text string) error {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	_, err := c.api.Send(edit)
	if err != nil {
		return fmt.Errorf("telegram edit: %w", err)
	}
	return nil
}

func (c *realClient) AnswerCallback(ctx context.Context, callbackID string, text string) error {
	ans := tgbotapi.NewCallback(callbackID, text)
	_, err := c.api.Request(ans)
	if err != nil {
		return fmt.Errorf("telegram answer callback: %w", err)
	}
	return nil
}

func (c *realClient) Updates(ctx context.Context) (<-chan Update, error) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	ch := c.api.GetUpdatesChan(u)

	out := make(chan Update)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				c.api.StopReceivingUpdates()
				return
			case up, ok := <-ch:
				if !ok {
					return
				}
				if up.CallbackQuery != nil {
					if up.CallbackQuery.From == nil || up.CallbackQuery.From.ID != c.allowedUserID {
						continue
					}
					out <- Update{
						UserID: up.CallbackQuery.From.ID,
						ChatID: up.CallbackQuery.Message.Chat.ID,
						Callback: &CallbackQuery{
							ID:        up.CallbackQuery.ID,
							MessageID: up.CallbackQuery.Message.MessageID,
							Data:      up.CallbackQuery.Data,
						},
					}
					continue
				}
				if up.Message == nil || up.Message.From == nil {
					continue
				}
				if up.Message.From.ID != c.allowedUserID {
					// Silently drop messages from any other user.
					continue
				}
				out <- Update{
					UserID: up.Message.From.ID,
					ChatID: up.Message.Chat.ID,
					Text:   up.Message.Text,
				}
			}
		}
	}()
	return out, nil
}
