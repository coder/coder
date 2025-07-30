package usagetypes

// Please read the package documentation before adding imports.
import (
	"encoding/json"
	"time"

	"golang.org/x/xerrors"
)

const (
	TallymanCoderLicenseKeyHeader   = "Coder-License-Key"
	TallymanCoderDeploymentIDHeader = "Coder-Deployment-ID"
)

// TallymanV1Response is a generic response with a message from the Tallyman
// API. It is typically returned when there is an error.
type TallymanV1Response struct {
	Message string `json:"message"`
}

// TallymanV1IngestRequest is a request to the Tallyman API to ingest usage
// events.
type TallymanV1IngestRequest struct {
	Events []TallymanV1IngestEvent `json:"events"`
}

// TallymanV1IngestEvent is an event to be ingested into the Tallyman API.
type TallymanV1IngestEvent struct {
	ID        string          `json:"id"`
	EventType UsageEventType  `json:"event_type"`
	EventData json.RawMessage `json:"event_data"`
	CreatedAt time.Time       `json:"created_at"`
}

// Valid validates the TallymanV1IngestEvent. It does not validate the event
// body.
func (e TallymanV1IngestEvent) Valid() error {
	if e.ID == "" {
		return xerrors.New("id is required")
	}
	if !e.EventType.Valid() {
		return xerrors.Errorf("event_type %q is invalid", e.EventType)
	}
	if e.CreatedAt.IsZero() {
		return xerrors.New("created_at cannot be zero")
	}
	return nil
}

// TallymanV1IngestResponse is a response from the Tallyman API to ingest usage
// events.
type TallymanV1IngestResponse struct {
	AcceptedEvents []TallymanV1IngestAcceptedEvent `json:"accepted_events"`
	RejectedEvents []TallymanV1IngestRejectedEvent `json:"rejected_events"`
}

// TallymanV1IngestAcceptedEvent is an event that was accepted by the Tallyman
// API.
type TallymanV1IngestAcceptedEvent struct {
	ID string `json:"id"`
}

// TallymanV1IngestRejectedEvent is an event that was rejected by the Tallyman
// API.
type TallymanV1IngestRejectedEvent struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Permanent bool   `json:"permanent"`
}
