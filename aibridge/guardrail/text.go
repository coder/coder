package guardrail

import (
	"strconv"

	"github.com/tidwall/gjson"
)

// UserPromptTexts returns the text span(s) to scan for a request body: the
// latest user prompt, addressed by an sjson pointer so a masking guardrail can
// write a redacted value back to the same location.
//
// Selection: the most recent message/input item with role "user" that carries a
// text block, and only that message's last text block (clients inject context
// ahead of the prompt). Trailing non-user turns are skipped, NOT treated as
// "no prompt": agentic clients append a mid-conversation system message and
// tool-result turns after the user's prompt on real requests, so requiring the
// literal last item to be a user message (as the interceptors do for prompt
// recording) would leave the user's PII unscanned. Scanning stops at the most
// recent user turn, so an older turn's content is never re-scanned.
//
// It handles the bridged request shapes:
//   - Anthropic Messages and OpenAI Chat Completions: top-level "messages".
//   - OpenAI Responses: top-level "input" (string or array of input items).
//
// Anthropic's top-level "system" prompt is intentionally not scanned (system
// content is not a user prompt).
func UserPromptTexts(body []byte) []TextRef {
	root := gjson.ParseBytes(body)

	// OpenAI Responses: "input" as a string or an array of input items.
	if input := root.Get("input"); input.Exists() {
		if input.Type == gjson.String {
			return single(TextRef{Pointer: "input", Role: RoleUser, Value: input.String()})
		}
		if input.IsArray() {
			items := input.Array()
			if ref, ok := lastUserItemText(items, "input"); ok {
				return single(ref)
			}
		}
		return nil
	}

	// Anthropic Messages / OpenAI Chat Completions: "messages".
	messages := root.Get("messages")
	if messages.IsArray() {
		if ref, ok := lastUserItemText(messages.Array(), "messages"); ok {
			return single(ref)
		}
	}
	return nil
}

// ConversationTexts returns every role-tagged text span in the request body,
// in document order, each addressed by its sjson pointer. Unlike
// [UserPromptTexts] it does not stop at the latest user turn and it includes
// system/assistant/tool text, so a scanner that needs full context
// (prompt-injection, jailbreak, secret detection) reads the whole conversation
// without re-parsing provider-specific shapes. Anthropic's top-level "system"
// string is included as a system span.
func ConversationTexts(body []byte) []TextRef {
	root := gjson.ParseBytes(body)
	var refs []TextRef

	// Anthropic top-level system prompt.
	if sys := root.Get("system"); sys.Type == gjson.String {
		refs = append(refs, TextRef{Pointer: "system", Role: RoleSystem, Value: sys.String()})
	}

	if input := root.Get("input"); input.Exists() {
		if input.Type == gjson.String {
			return append(refs, TextRef{Pointer: "input", Role: RoleUser, Value: input.String()})
		}
		if input.IsArray() {
			return append(refs, itemTexts(input.Array(), "input")...)
		}
		return refs
	}

	if messages := root.Get("messages"); messages.IsArray() {
		refs = append(refs, itemTexts(messages.Array(), "messages")...)
	}
	return refs
}

// itemTexts collects every text block of every message item, tagged by role.
func itemTexts(items []gjson.Result, arrayPath string) []TextRef {
	var refs []TextRef
	for idx, item := range items {
		role := roleOf(item.Get("role").String())
		base := arrayPath + "." + strconv.Itoa(idx) + ".content"
		content := item.Get("content")
		switch {
		case content.Type == gjson.String:
			refs = append(refs, TextRef{Pointer: base, Role: role, Value: content.String()})
		case content.IsArray():
			for j, part := range content.Array() {
				if t := part.Get("text"); t.Type == gjson.String {
					refs = append(refs, TextRef{
						Pointer: base + "." + strconv.Itoa(j) + ".text",
						Role:    role,
						Value:   t.String(),
					})
				}
			}
		}
	}
	return refs
}

func roleOf(s string) Role {
	switch Role(s) {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return Role(s)
	default:
		return RoleUser
	}
}

// lastUserItemText walks items from the end and returns the text ref for the
// most recent role "user" item that carries a text block, using arrayPath as
// the sjson path of items (e.g. "messages" or "input"). Non-user items and
// user items without a text block (e.g. tool-result turns) are skipped.
func lastUserItemText(items []gjson.Result, arrayPath string) (TextRef, bool) {
	for idx := len(items) - 1; idx >= 0; idx-- {
		item := items[idx]
		if item.Get("role").String() != "user" {
			continue
		}
		prefix := arrayPath + "." + strconv.Itoa(idx) + ".content"
		if ref, ok := lastTextBlock(item.Get("content"), prefix); ok {
			return ref, true
		}
	}
	return TextRef{}, false
}

// lastTextBlock returns the text ref for a message's content: the string itself
// when content is a string, or the last array element carrying a string "text"
// field (clients inject context ahead of the prompt, so the final text block is
// the user's actual prompt). pointer is the sjson path of the content node.
func lastTextBlock(content gjson.Result, pointer string) (TextRef, bool) {
	switch {
	case content.Type == gjson.String:
		return TextRef{Pointer: pointer, Role: RoleUser, Value: content.String()}, true
	case content.IsArray():
		parts := content.Array()
		for j := len(parts) - 1; j >= 0; j-- {
			if t := parts[j].Get("text"); t.Type == gjson.String {
				return TextRef{
					Pointer: pointer + "." + strconv.Itoa(j) + ".text",
					Role:    RoleUser,
					Value:   t.String(),
				}, true
			}
		}
	}
	return TextRef{}, false
}

func single(ref TextRef) []TextRef { return []TextRef{ref} }
