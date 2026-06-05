package intercept

import "github.com/tidwall/gjson"

// Payload is a request body read once and shared across the gateway: policy
// evaluation, session detection, and interceptor creation all consume it rather
// than re-reading r.Body. Route and transform policies replace it via WithBody.
type Payload struct {
	raw []byte
}

// NewPayload wraps an already-read request body.
func NewPayload(body []byte) Payload { return Payload{raw: body} }

// Body returns the raw JSON request body.
func (p Payload) Body() []byte { return p.raw }

// WithBody returns a copy of the payload with the body replaced.
func (p Payload) WithBody(body []byte) Payload { return Payload{raw: body} }

// Model returns the top-level "model" field, or "" when absent. Both supported
// providers carry the model at the body root.
func (p Payload) Model() string { return gjson.GetBytes(p.raw, "model").String() }

// Stream returns the top-level "stream" boolean.
func (p Payload) Stream() bool { return gjson.GetBytes(p.raw, "stream").Bool() }
