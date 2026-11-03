package codersdk

import "encoding/json"

// GroupSyncWarning reports one finding in group sync settings that the server
// accepted, and what it did about it. Warnings accompany the settings in API
// responses, so that a client can compare what it sent with what was saved.
type GroupSyncWarning struct {
	// Code names the condition violated.
	Code GroupSyncFindingCode `json:"code"`
	// Path is a JSON Pointer (RFC 6901) to the element concerned, in the
	// settings as received. The empty path refers to the whole text.
	Path string `json:"path"`
	// Received is the JSON text of the element at Path, as received.
	Received json.RawMessage `json:"received"`
	// Accepted is the JSON text of the element in the settings as saved or
	// returned. It is absent when the element was removed, and equal to
	// Received when the settings were not rewritten.
	Accepted json.RawMessage `json:"accepted,omitempty"`
}
