package agentnative

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/xerrors"
)

const openCodeCanceledStatus = "cancelled" //nolint:misspell // Preserve the durable receipt format.

type openCodeSnapshotKind uint8

const (
	openCodeLiveSnapshot openCodeSnapshotKind = iota
	openCodeResumeSnapshot
)

// Receipts are committed with events by Host.Publish. They are not native
// history and must never be submitted as a conversation on resume.
type openCodeReceipts struct {
	Version   int                                `json:"version"`
	LastIdle  string                             `json:"lastIdle,omitempty"`
	Cache     map[string]*openCodeMessageReceipt `json:"cache"`
	Turns     []string                           `json:"turns,omitempty"`
	Active    string                             `json:"active,omitempty"`
	Requests  map[string]*openCodeRequestReceipt `json:"requests"`
	Approvals []string                           `json:"approvals,omitempty"`
}

type openCodeMessageReceipt struct {
	Type  string                       `json:"type"`
	Hash  string                       `json:"hash,omitempty"`
	Turn  string                       `json:"turn,omitempty"`
	Parts map[int]*openCodePartReceipt `json:"parts,omitempty"`
}

type openCodePartReceipt struct {
	Type   string `json:"type"`
	Length int    `json:"length,omitempty"`
	Hash   string `json:"hash,omitempty"`
	ID     string `json:"id,omitempty"`
	Done   bool   `json:"done,omitempty"`
}

type openCodeRequestReceipt struct {
	NativeID   string `json:"nativeId"`
	Hash       string `json:"hash"`
	Status     string `json:"status"`
	CancelSent bool   `json:"cancelSent,omitempty"`
	CancelBy   int64  `json:"cancelBy,omitempty"`
}

type openCodeMessage struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Text    *string        `json:"text"`
	Content []openCodePart `json:"content"`
	Outcome string         `json:"outcome"`
	Status  string         `json:"status"`
}

type openCodePart struct {
	Type  string  `json:"type"`
	Text  *string `json:"text"`
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	State struct {
		Status  string `json:"status"`
		Input   any    `json:"input"`
		Content any    `json:"content"`
	} `json:"state"`
}

type openCodeMessagePage struct {
	Data   []openCodeMessage `json:"data"`
	Cursor *struct {
		Previous *string `json:"previous"`
		Next     *string `json:"next"`
	} `json:"cursor"`
}

func openCodeHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func readOpenCodeReceipts(data json.RawMessage) (*openCodeReceipts, error) {
	r := &openCodeReceipts{Version: 1, Cache: map[string]*openCodeMessageReceipt{}, Requests: map[string]*openCodeRequestReceipt{}}
	if len(data) == 0 {
		return r, nil
	}
	if len(data) > 256<<10 || json.Unmarshal(data, r) != nil || r.Version != 1 || r.Cache == nil || r.Requests == nil || len(r.Cache) > 200 || len(r.Requests) > 1000 || len(r.Approvals) > 256 || len(r.Turns) > 200 {
		return nil, errOpenCodeProtocol
	}
	if r.LastIdle != "" && !openCodeMessageID.MatchString(r.LastIdle) {
		return nil, errOpenCodeProtocol
	}
	owned := map[string]bool{}
	for id, request := range r.Requests {
		if id == "" || len(id) > 200 || request == nil || !openCodeMessageID.MatchString(request.NativeID) || !openCodeDigest.MatchString(request.Hash) || owned[request.NativeID] || !slices.Contains([]string{"submitting", "queued", "running", "completed", openCodeCanceledStatus, "rejected"}, request.Status) {
			return nil, errOpenCodeProtocol
		}
		owned[request.NativeID] = true
	}
	for id, message := range r.Cache {
		if !openCodeMessageID.MatchString(id) || message == nil {
			return nil, errOpenCodeProtocol
		}
		switch message.Type {
		case "user", "synthetic":
			if !openCodeDigest.MatchString(message.Hash) || !slices.Contains(r.Turns, id) {
				return nil, errOpenCodeProtocol
			}
		case "assistant":
			if message.Parts == nil {
				message.Parts = map[int]*openCodePartReceipt{}
			}
			if !slices.Contains(r.Turns, message.Turn) || len(message.Parts) > 1024 {
				return nil, errOpenCodeProtocol
			}
			for index, part := range message.Parts {
				if index < 0 || index >= 1024 || part == nil {
					return nil, errOpenCodeProtocol
				}
				switch part.Type {
				case "text", "reasoning":
					if part.Length < 0 || part.Length > openCodeMaxResponse || !openCodeDigest.MatchString(part.Hash) {
						return nil, errOpenCodeProtocol
					}
				case "tool":
					if part.ID == "" || len(part.ID) > 200 {
						return nil, errOpenCodeProtocol
					}
				default:
					return nil, errOpenCodeProtocol
				}
			}
		default:
			return nil, errOpenCodeProtocol
		}
	}
	seen := map[string]bool{}
	for _, id := range r.Turns {
		m := r.Cache[id]
		if m == nil || (m.Type != "user" && m.Type != "synthetic") || seen[id] {
			return nil, errOpenCodeProtocol
		}
		seen[id] = true
	}
	if r.Active != "" && !seen[r.Active] {
		return nil, errOpenCodeProtocol
	}
	return r, nil
}

func (p openCodeMessagePage) valid(limit int) bool {
	if p.Data == nil || len(p.Data) > limit || p.Cursor == nil {
		return false
	}
	for _, cursor := range []*string{p.Cursor.Next, p.Cursor.Previous} {
		if cursor != nil && (len(*cursor) == 0 || len(*cursor) > 4096) {
			return false
		}
	}
	return true
}

func (r *openCodeReceipts) owner(nativeID string) string {
	for id, request := range r.Requests {
		if request.NativeID == nativeID {
			return id
		}
	}
	return ""
}

func (r *openCodeReceipts) event(kind, turn string, data map[string]any) Event {
	if turn != "" {
		data["turnId"] = turn
		if owner := r.owner(turn); owner != "" {
			data["requestId"] = owner
		}
	}
	return Event{Type: kind, Data: data}
}

func openCodeChunkEnd(text string, start int) int {
	end := min(len(text), start+16000)
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	return end
}

// project validates the entire snapshot before callers publish any output.
func (r *openCodeReceipts) project(page openCodeMessagePage, kind openCodeSnapshotKind) ([]Event, bool, error) {
	if !page.valid(200) {
		return nil, false, errOpenCodeProtocol
	}
	ordered := slices.Clone(page.Data)
	slices.Reverse(ordered)
	ids := map[string]string{}
	boundary := -1
	for index, message := range ordered {
		if !openCodeMessageID.MatchString(message.ID) || ids[message.ID] != "" || message.Type == "" {
			return nil, false, errOpenCodeProtocol
		}
		ids[message.ID] = message.Type
		if message.ID == r.LastIdle {
			boundary = index
			if message.Type != "idle" {
				return nil, false, errOpenCodeProtocol
			}
		}
	}
	if r.LastIdle != "" && boundary < 0 {
		return nil, false, errOpenCodeAnchor
	}
	pending := ordered[boundary+1:]
	visible := map[string]string{}
	for _, message := range pending {
		visible[message.ID] = message.Type
	}
	for id, receipt := range r.Cache {
		if visible[id] != receipt.Type {
			return nil, false, errOpenCodeAnchor
		}
	}
	var events []Event
	if kind == openCodeResumeSnapshot {
		for _, turn := range r.Turns {
			m := r.Cache[turn]
			events = append(events, r.event("turn.start", turn, map[string]any{"text": "", "nativeMessageId": turn, "nativeMessageType": m.Type, "nativeTextHash": m.Hash}))
		}
	}
	active := ""
	background := false
	for _, message := range pending {
		old := r.Cache[message.ID]
		switch message.Type {
		case "user", "synthetic":
			if message.Text == nil {
				return nil, false, errOpenCodeProtocol
			}
			hash := openCodeHash(*message.Text)
			owner := r.owner(message.ID)
			if message.Type == "synthetic" && owner != "" {
				return nil, false, errOpenCodeProtocol
			}
			if old != nil && old.Hash != hash {
				return nil, false, errOpenCodeProtocol
			}
			if owner != "" && r.Requests[owner].Hash != hash {
				return nil, false, errOpenCodeProtocol
			}
			if old == nil {
				if owner == "" {
					events = append(events, Event{Type: "mode", Data: map[string]any{"mode": "human"}})
				}
				r.Cache[message.ID] = &openCodeMessageReceipt{Type: message.Type, Hash: hash}
				r.Turns = append(r.Turns, message.ID)
				for start := 0; ; {
					end := openCodeChunkEnd(*message.Text, start)
					events = append(events, r.event("turn.start", message.ID, map[string]any{"text": (*message.Text)[start:end], "nativeMessageId": message.ID, "nativeMessageType": message.Type, "nativeTextHash": hash}))
					if end == len(*message.Text) {
						break
					}
					start = end
				}
			}
			active = message.ID
			if owner != "" {
				request := r.Requests[owner]
				if request.Status == openCodeCanceledStatus || request.Status == "rejected" || request.Status == "completed" {
					return nil, false, errOpenCodeProtocol
				}
				request.Status = "running"
			}
		case "assistant":
			if message.Content == nil || len(message.Content) > 1024 || active == "" {
				return nil, false, errOpenCodeProtocol
			}
			if old == nil {
				old = &openCodeMessageReceipt{Type: "assistant", Turn: active, Parts: map[int]*openCodePartReceipt{}}
				r.Cache[message.ID] = old
			}
			if old.Turn != active || len(old.Parts) > len(message.Content) {
				return nil, false, errOpenCodeProtocol
			}
			for index, part := range message.Content {
				previous := old.Parts[index]
				if previous != nil && previous.Type != part.Type {
					return nil, false, errOpenCodeProtocol
				}
				meta := map[string]any{"nativeMessageId": message.ID, "nativePartIndex": index}
				switch part.Type {
				case "text", "reasoning":
					if part.Text == nil {
						return nil, false, errOpenCodeProtocol
					}
					if previous == nil {
						previous = &openCodePartReceipt{Type: part.Type, Hash: openCodeHash("")}
					}
					if len(*part.Text) < previous.Length || !utf8.ValidString((*part.Text)[:previous.Length]) || openCodeHash((*part.Text)[:previous.Length]) != previous.Hash {
						return nil, false, xerrors.New("OpenCode output prefix changed; no work was replayed")
					}
					for previous.Length < len(*part.Text) {
						end := openCodeChunkEnd(*part.Text, previous.Length)
						data := maps.Clone(meta)
						data["text"] = (*part.Text)[previous.Length:end]
						previous.Length = end
						previous.Hash = openCodeHash((*part.Text)[:end])
						data["nativeTextLength"] = end
						data["nativeTextHash"] = previous.Hash
						kind := "text"
						if part.Type == "reasoning" {
							kind = "thought"
						}
						events = append(events, r.event(kind, active, data))
					}
					old.Parts[index] = previous
				case "tool":
					if part.ID == "" || len(part.ID) > 200 || part.Name == "" || len(part.Name) > 200 || !slices.Contains([]string{"streaming", "running", "completed", "error"}, part.State.Status) {
						return nil, false, errOpenCodeProtocol
					}
					done := part.State.Status == "completed" || part.State.Status == "error"
					background = background || !done
					if previous == nil {
						data := maps.Clone(meta)
						data["toolCallId"] = part.ID
						data["title"] = part.Name
						data["input"] = part.State.Input
						events = append(events, r.event("tool.start", active, data))
						previous = &openCodePartReceipt{Type: "tool", ID: part.ID}
						old.Parts[index] = previous
					}
					if previous.ID != part.ID || (previous.Done && !done) {
						return nil, false, errOpenCodeProtocol
					}
					if done && !previous.Done {
						data := maps.Clone(meta)
						data["toolCallId"] = part.ID
						data["isError"] = part.State.Status == "error"
						data["input"] = part.State.Input
						data["result"] = part.State.Content
						events = append(events, r.event("tool.end", active, data))
						previous.Done = true
					}
				default:
					return nil, false, errOpenCodeProtocol
				}
			}
		case "idle":
			reason := map[string]string{"succeeded": "answer", "failed": "error", "interrupted": "aborted"}[message.Outcome]
			if reason == "" {
				return nil, false, errOpenCodeProtocol
			}
			for _, turn := range r.Turns {
				events = append(events, r.event("turn.complete", turn, map[string]any{"nativeMessageId": message.ID, "answer": "", "reason": reason}))
				if owner := r.owner(turn); owner != "" {
					r.Requests[owner].Status = "completed"
				}
			}
			r.LastIdle = message.ID
			clear(r.Cache)
			r.Turns = nil
			active = ""
			background = false
		case "shell":
			background = background || message.Status == "running"
		case "agent-switched", "model-switched", "location-switched", "system", "skill", "compaction":
		default:
			return nil, false, errOpenCodeProtocol
		}
		if len(r.Cache) > 200 {
			return nil, false, errOpenCodeProtocol
		}
	}
	r.Active = active
	return events, background, nil
}

func openCodeRequestActive(request *openCodeRequestReceipt) bool {
	return request.Status == "submitting" || request.Status == "queued" || request.Status == "running"
}

func openCodeValidatePrompt(prompt Prompt) bool {
	return prompt.ID != "" && len(prompt.ID) <= 200 && len(prompt.Text) <= 400000 && utf8.ValidString(prompt.Text) && strings.TrimSpace(prompt.Text) != ""
}
