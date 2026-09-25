package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
)

// ParseResponseFormat parses the raw response_format member of a chat
// request strictly, with exact, unique keys at both object levels. An
// ordinary request (omitted, JSON null or {"type":"text"}) returns nil;
// otherwise it returns the encoded request part under a new server-minted
// request ID. enabled reports whether the chat-structured-output experiment
// accepts formats. A rejection names the exact field and never echoes schema
// values.
func ParseResponseFormat(raw json.RawMessage, enabled bool) (*codersdk.ChatMessagePart, *codersdk.ValidationError) {
	if raw = bytes.TrimSpace(raw); len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	reject := func(field, detail string) (*codersdk.ChatMessagePart, *codersdk.ValidationError) {
		return nil, &codersdk.ValidationError{Field: field, Detail: detail}
	}
	format, ok := jsonObjectMembers(raw, "type", "json_schema")
	var typ string
	typeErr := json.Unmarshal(format["type"], &typ)
	if ok && len(format) == 1 && typeErr == nil && typ == string(codersdk.ChatResponseFormatTypeText) {
		return nil, nil
	}
	switch {
	case !enabled:
		return reject("response_format", "Structured output is disabled. Enable the chat-structured-output experiment.")
	case !ok:
		return reject("response_format", "Must be a JSON object with unique keys, holding only type and json_schema.")
	case typeErr == nil && typ == string(codersdk.ChatResponseFormatTypeText):
		return reject("response_format", "json_schema requires type json_schema.")
	case typeErr != nil || typ != string(codersdk.ChatResponseFormatTypeJSONSchema):
		return reject("response_format.type", `Must be "text" or "json_schema".`)
	}
	spec, ok := jsonObjectMembers(format["json_schema"], "name", "description", "schema")
	if !ok {
		return reject("response_format.json_schema", "Must be an object with unique keys, holding only name, description and schema.")
	}
	var name, description string
	if json.Unmarshal(spec["name"], &name) != nil || !chatstructured.ValidRequestName(name) {
		return reject("response_format.json_schema.name", "Must match ^[A-Za-z0-9_-]{1,64}$.")
	}
	if raw, present := spec["description"]; present && (!utf8.Valid(raw) || json.Unmarshal(raw, &description) != nil || len(description) > 1024) {
		return reject("response_format.json_schema.description", "Must be a string of at most 1024 bytes of valid UTF-8.")
	}
	schema := bytes.TrimSpace(spec["schema"])
	if len(schema) == 0 || schema[0] != '{' {
		return reject("response_format.json_schema.schema", "Must be a JSON Schema object.")
	}
	if _, err := chatstructured.CompileSchema(schema); err != nil {
		return reject("response_format.json_schema.schema", "Must be a supported JSON Schema within the size and complexity limits.")
	}
	part, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: uuid.New(), Name: name, Description: description, Schema: schema})
	if err != nil {
		return reject("response_format.json_schema.schema", "Is too large to store.")
	}
	return &part, nil
}

// jsonObjectMembers splits a JSON object into its raw members, reporting
// false for any other value, a key outside allowed, or a repeated key, which
// lenient decoding would silently resolve.
func jsonObjectMembers(raw json.RawMessage, allowed ...string) (map[string]json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	members := make(map[string]json.RawMessage)
	for dec.More() {
		tok, err := dec.Token()
		key, isKey := tok.(string)
		if _, seen := members[key]; err != nil || !isKey || seen || !slices.Contains(allowed, key) {
			return nil, false
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, false
		}
		members[key] = value
	}
	_, err := dec.Token()
	return members, err == nil && !dec.More()
}

// HasPendingStructuredRequest reports whether the chat's latest user turn
// has an open structured output request or a queued message carries one.
func (p *Server) HasPendingStructuredRequest(ctx context.Context, chatID uuid.UUID) (bool, error) {
	history, err := p.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	if err != nil {
		return false, xerrors.Errorf("load chat messages: %w", err)
	}
	if _, open := openStructuredRequest(ctx, p.logger, chatID, history); open {
		return true, nil
	}
	queued, err := p.db.GetChatQueuedMessages(ctx, chatID)
	if err != nil {
		return false, xerrors.Errorf("load queued messages: %w", err)
	}
	for _, q := range queued {
		if hasStructuredRequestPart(queuedRow(q)) {
			return true, nil
		}
	}
	return false, nil
}

// hasStructuredRequestPart reports whether msg holds a request part, valid or not.
func hasStructuredRequestPart(msg database.ChatMessage) bool {
	if !bytes.Contains(msg.Content.RawMessage, []byte(codersdk.ChatMessagePartTypeStructuredOutputRequest)) {
		return false
	}
	parts, err := chatprompt.ParseContent(msg)
	return err != nil || slices.ContainsFunc(parts, func(part codersdk.ChatMessagePart) bool {
		return part.Type == codersdk.ChatMessagePartTypeStructuredOutputRequest
	})
}
