package agenttoolcall

import "github.com/google/uuid"

// Canceled reports whether tool call id of chat chatID is marked
// canceled.
func (t *Table) Canceled(chatID, id uuid.UUID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[key{chatID: chatID, id: id}]
	return ok && e.canceled
}
