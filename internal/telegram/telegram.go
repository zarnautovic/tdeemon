// Package telegram sends messages through a Telegram bot: the morning
// summary and the watchdog's alarms. It only sends; it reads no updates,
// so another program may poll the same bot.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf16"
)

// DefaultBaseURL is Telegram's Bot API.
const DefaultBaseURL = "https://api.telegram.org"

// MaxText is the longest message Telegram takes, in UTF-16 code units,
// as it counts lengths: an emoji takes two.
const MaxText = 4096

// timeout bounds a send, so a Telegram that does not answer does not hold
// the scheduler.
const timeout = 15 * time.Second

// Bot sends messages as the bot with Token.
type Bot struct {
	// Token is the bot's token from BotFather. It is part of every
	// request's URL, so no error carries the URL.
	Token string
	// BaseURL is the Bot API's; DefaultBaseURL when empty.
	BaseURL string
	// Client is the HTTP client; one with a timeout when nil.
	Client *http.Client
}

// Error is Telegram's answer to a request it refused, such as a chat
// that blocked the bot (403) or too many requests (429).
type Error struct {
	Code        int
	Description string
	// RetryAfter is how long Telegram asks to wait, for a 429.
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	return fmt.Sprintf("telegram: %d %s", e.Code, e.Description)
}

// Send sends text to the chat chatID, as plain text without link
// previews. Text longer than MaxText is cut, ending with an ellipsis. No
// error it returns carries the token.
func (b *Bot) Send(ctx context.Context, chatID int64, text string) error {
	if err := b.send(ctx, chatID, text); err != nil {
		if b.Token != "" && strings.Contains(err.Error(), b.Token) {
			// A transport of a custom Client may name the URL.
			return errors.New(strings.ReplaceAll(err.Error(), b.Token, "[redacted]"))
		}
		return err
	}
	return nil
}

func (b *Bot) send(ctx context.Context, chatID int64, text string) error {
	if b.Token == "" {
		return errors.New("telegram: no bot token")
	}
	if chatID == 0 {
		return errors.New("telegram: no chat")
	}
	body, err := json.Marshal(map[string]any{
		"chat_id":              chatID,
		"text":                 cut(text),
		"link_preview_options": map[string]bool{"is_disabled": true},
	})
	if err != nil {
		return err
	}
	base := b.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(base, "/")+"/bot"+b.Token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("telegram: a request that does not build") // its error would carry the token
	}
	req.Header.Set("Content-Type", "application/json")
	client := b.Client
	if client == nil {
		// The Bot API does not redirect; a redirect would take the POST,
		// and the token in its URL, elsewhere.
		client = &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("a redirect")
		}}
	}
	resp, err := client.Do(req)
	if err != nil {
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err // without the URL, which holds the token
		}
		return fmt.Errorf("telegram: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var answer struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&answer); err != nil {
		return fmt.Errorf("telegram: an answer of %d that does not read: %w", resp.StatusCode, err)
	}
	if !answer.OK {
		code := answer.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}
		return &Error{Code: code, Description: answer.Description, RetryAfter: time.Duration(answer.Parameters.RetryAfter) * time.Second}
	}
	return nil
}

// cut returns text cut to MaxText UTF-16 code units, the last an
// ellipsis, at a character's end.
func cut(text string) string {
	if units(text) <= MaxText {
		return text
	}
	n := 0
	for i, r := range text {
		if n+utf16.RuneLen(r) > MaxText-1 {
			return text[:i] + "…"
		}
		n += utf16.RuneLen(r)
	}
	return text
}

// units counts text's UTF-16 code units.
func units(text string) int {
	n := 0
	for _, r := range text {
		n += utf16.RuneLen(r)
	}
	return n
}
