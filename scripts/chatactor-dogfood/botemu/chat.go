package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

const systemPrompt = "You are a test harness. When the user names a tool, call exactly that tool once with the arguments they give, then reply with the tool's raw output verbatim. If a tool returns an error, reply with the error text verbatim. Never call any other tool."

// turnResult summarizes what a turn produced after the posted user message.
type turnResult struct {
	UserMessage codersdk.ChatMessage
	Chat        codersdk.Chat
	// Messages are all messages stored after the user message.
	Messages []codersdk.ChatMessage
}

// toolResults returns every tool-result part after the user message.
func (t turnResult) toolResults() []codersdk.ChatMessagePart {
	var out []codersdk.ChatMessagePart
	for _, m := range t.Messages {
		for _, p := range m.Content {
			if p.Type == codersdk.ChatMessagePartTypeToolResult {
				out = append(out, p)
			}
		}
	}
	return out
}

// toolCalls returns every tool-call part after the user message.
func (t turnResult) toolCalls() []codersdk.ChatMessagePart {
	var out []codersdk.ChatMessagePart
	for _, m := range t.Messages {
		for _, p := range m.Content {
			if p.Type == codersdk.ChatMessagePartTypeToolCall {
				out = append(out, p)
			}
		}
	}
	return out
}

// assistantText joins the assistant text parts after the user message.
func (t turnResult) assistantText() string {
	var b strings.Builder
	for _, m := range t.Messages {
		if m.Role != codersdk.ChatMessageRoleAssistant {
			continue
		}
		for _, p := range m.Content {
			if p.Type == codersdk.ChatMessagePartTypeText {
				_, _ = b.WriteString(p.Text)
				_, _ = b.WriteString(" ")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// resultText flattens a tool-result JSON payload to a searchable string.
func resultText(p codersdk.ChatMessagePart) string {
	if len(p.Result) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(p.Result, &s); err == nil {
		return s
	}
	return string(p.Result)
}

// postAndWait posts a text prompt as the given client and waits until the
// turn settles, then returns the stored user message and everything after.
func postAndWait(ctx context.Context, c *codersdk.Client, chatID uuid.UUID, text string, timeout time.Duration) (turnResult, error) {
	resp, err := c.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: text}},
	})
	if err != nil {
		return turnResult{}, err
	}
	if resp.Message == nil {
		return turnResult{}, xerrors.Errorf("post returned no message (queued=%v)", resp.Queued)
	}
	return waitTurn(ctx, c, chatID, *resp.Message, timeout)
}

func waitTurn(ctx context.Context, c *codersdk.Client, chatID uuid.UUID, userMsg codersdk.ChatMessage, timeout time.Duration) (turnResult, error) {
	deadline := time.Now().Add(timeout)
	sawRunning := false
	for time.Now().Before(deadline) {
		time.Sleep(1500 * time.Millisecond)
		chat, err := c.GetChat(ctx, chatID)
		if err != nil {
			return turnResult{}, xerrors.Errorf("get chat: %w", err)
		}
		switch chat.Status {
		case codersdk.ChatStatusRunning, codersdk.ChatStatusInterrupting:
			sawRunning = true
			continue
		}
		msgs, err := c.GetChatMessages(ctx, chatID, &codersdk.ChatMessagesPaginationOptions{AfterID: userMsg.ID, Limit: 100})
		if err != nil {
			return turnResult{}, xerrors.Errorf("get messages: %w", err)
		}
		if len(msgs.Messages) == 0 && !sawRunning && chat.Status == codersdk.ChatStatusWaiting {
			// The turn may not have started yet.
			continue
		}
		if len(msgs.Messages) == 0 && chat.Status == codersdk.ChatStatusWaiting && time.Since(userMsg.CreatedAt) < 10*time.Second {
			continue
		}
		return turnResult{UserMessage: userMsg, Chat: chat, Messages: msgs.Messages}, nil
	}
	chat, _ := c.GetChat(ctx, chatID)
	return turnResult{UserMessage: userMsg, Chat: chat}, xerrors.Errorf("turn did not settle within %s (status=%s)", timeout, chat.Status)
}

// describeTurn renders a compact evidence block for a turn.
func describeTurn(t turnResult) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "user_message id=%d created_by=%s; chat status=%s", t.UserMessage.ID, uuidStr(t.UserMessage.CreatedBy), t.Chat.Status)
	if t.Chat.LastError != nil {
		_, _ = fmt.Fprintf(&b, " last_error=%q", t.Chat.LastError.Message)
	}
	for _, call := range t.toolCalls() {
		_, _ = fmt.Fprintf(&b, "\n  tool-call %s args=%s", call.ToolName, truncate(string(call.Args), 200))
	}
	for _, res := range t.toolResults() {
		_, _ = fmt.Fprintf(&b, "\n  tool-result %s is_error=%v: %s", res.ToolName, res.IsError, truncate(resultText(res), 400))
	}
	_, _ = fmt.Fprintf(&b, "\n  assistant: %s", truncate(t.assistantText(), 400))
	return b.String()
}

func uuidStr(u *uuid.UUID) string {
	if u == nil {
		return "<nil>"
	}
	return u.String()
}
