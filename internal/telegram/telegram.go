// Package telegram talks to Telegram as a bot: it sends messages (the
// morning summary, the watchdog's alarms, answers) and reads the bot's
// updates, the messages its chats send it. Telegram gives a bot's
// updates to one program at a time, so no other program may poll the
// same bot.
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
// the scheduler; a request for updates gets it on top of its wait.
const timeout = 15 * time.Second

// Bot talks to Telegram as the bot with Token.
type Bot struct {
	// Token is the bot's token from BotFather. It is part of every
	// request's URL, so no error carries the URL.
	Token string
	// BaseURL is the Bot API's; DefaultBaseURL when empty.
	BaseURL string
	// Client is the HTTP client; one with a timeout when nil. One without
	// a CheckRedirect is used as if it refused redirects.
	Client *http.Client
}

// Error is Telegram's answer to a request it refused, such as a chat
// that blocked the bot (403) or too many requests (429).
type Error struct {
	Code        int
	Description string
	// RetryAfter is how long Telegram asks to wait, for a 429.
	RetryAfter time.Duration
	// MigrateTo is the chat a group became, as a supergroup: the chat to
	// send to instead.
	MigrateTo int64
}

func (e *Error) Error() string {
	return fmt.Sprintf("telegram: %d %s", e.Code, e.Description)
}

// Send sends text to the chat chatID, as plain text without link
// previews. Text longer than MaxText is cut, ending with an ellipsis. No
// error it returns carries the token.
func (b *Bot) Send(ctx context.Context, chatID int64, text string) error {
	if chatID == 0 {
		return errors.New("telegram: no chat")
	}
	return b.redact(b.call(ctx, "sendMessage", map[string]any{
		"chat_id":              chatID,
		"text":                 cut(text),
		"link_preview_options": map[string]bool{"is_disabled": true},
	}, nil, timeout))
}

// Command is one of the bot's commands, as Telegram's menu lists it.
type Command struct {
	// Name is the command without its slash, such as "help".
	Name        string
	Description string
}

// SetCommands sets the commands Telegram's menu lists for the bot to a
// user whose app is in language (a code such as "hr"), or, for "", to
// every user without commands in theirs. No error it returns carries the
// token.
func (b *Bot) SetCommands(ctx context.Context, commands []Command, language string) error {
	list := make([]map[string]string, 0, len(commands))
	for _, c := range commands {
		list = append(list, map[string]string{"command": c.Name, "description": c.Description})
	}
	params := map[string]any{"commands": list}
	if language != "" {
		params["language_code"] = language
	}
	return b.redact(b.call(ctx, "setMyCommands", params, nil, timeout))
}

// Update is a message to the bot, from Telegram's updates.
type Update struct {
	// ID is the update's, which orders them: the next request asks for
	// the updates from the last one's ID + 1.
	ID int64
	// ChatID is the chat the message came in; 0 for an update that is
	// not a message, which has nothing to answer.
	ChatID int64
	Text   string
	// Sent is when the message was sent, to the second.
	Sent time.Time
}

// Updates returns the bot's messages from the update offset on, waiting
// up to wait for one to come; none when none came. Asking from an
// offset confirms the updates before it, which Telegram then no longer
// gives. No error it returns carries the token.
func (b *Bot) Updates(ctx context.Context, offset int64, wait time.Duration) ([]Update, error) {
	var result []struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			Date int64  `json:"date"`
			Text string `json:"text"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	}
	err := b.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         int(wait / time.Second),
		"allowed_updates": []string{"message"},
	}, &result, wait+timeout)
	if err != nil {
		return nil, b.redact(err)
	}
	updates := make([]Update, 0, len(result))
	for _, r := range result {
		u := Update{ID: r.UpdateID}
		if r.Message != nil {
			u.ChatID, u.Text, u.Sent = r.Message.Chat.ID, r.Message.Text, time.Unix(r.Message.Date, 0)
		}
		updates = append(updates, u)
	}
	return updates, nil
}

// redact returns err with the token taken out of its message. A
// transport of a custom Client, or a Bot API of one's own, may name the
// URL: the token goes, the error's type and chain stay.
func (b *Bot) redact(err error) error {
	if err == nil || b.Token == "" || !strings.Contains(err.Error(), b.Token) {
		return err
	}
	if te, ok := errors.AsType[*Error](err); ok {
		redacted := *te
		redacted.Description = strings.ReplaceAll(te.Description, b.Token, "[redacted]")
		return &redacted
	}
	return &redactedError{msg: strings.ReplaceAll(err.Error(), b.Token, "[redacted]"), err: err}
}

// redactedError is an error whose message has the token taken out; its
// chain is the error's.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// call calls the Bot API's method with params, within limit, and decodes
// its result into result, unless nil.
func (b *Bot) call(ctx context.Context, method string, params, result any, limit time.Duration) error {
	if b.Token == "" {
		return errors.New("telegram: no bot token")
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	base := b.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(base, "/")+"/bot"+b.Token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return errors.New("telegram: a request that does not build") // its error would carry the token
	}
	req.Header.Set("Content-Type", "application/json")
	// The Bot API does not redirect; a redirect would take the POST, and
	// the token in its URL, elsewhere. A Client that does not refuse them
	// gets a copy that does.
	client := &http.Client{Timeout: limit}
	if b.Client != nil {
		c := *b.Client
		client = &c
	}
	if client.CheckRedirect == nil {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("a redirect") }
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
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
		Parameters  struct {
			RetryAfter int   `json:"retry_after"`
			MigrateTo  int64 `json:"migrate_to_chat_id"`
		} `json:"parameters"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&answer); err != nil {
		return fmt.Errorf("telegram: an answer of %d that does not read: %w", resp.StatusCode, err)
	}
	if !answer.OK {
		code := answer.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}
		return &Error{
			Code: code, Description: answer.Description,
			RetryAfter: time.Duration(answer.Parameters.RetryAfter) * time.Second, MigrateTo: answer.Parameters.MigrateTo,
		}
	}
	if result != nil {
		if err := json.Unmarshal(answer.Result, result); err != nil {
			return fmt.Errorf("telegram: a result of %s that does not read: %w", method, err)
		}
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
