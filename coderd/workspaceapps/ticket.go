package workspaceapps

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"gopkg.in/square/go-jose.v2"
)

const ticketSigningAlgorithm = jose.HS512

// Ticket is the struct data contained inside a workspace app ticket JWE. It
// contains the details of the workspace app that the ticket is valid for to
// avoid database queries.
type Ticket struct {
	// Request details.
	Request `json:"request"`

	// Trusted resolved details.
	Expiry      time.Time `json:"expiry"` // set by GenerateTicket if unset
	UserID      uuid.UUID `json:"user_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	AppURL      string    `json:"app_url"`
}

// MatchesRequest returns true if the ticket matches the request. Any ticket
// that does not match the request should be considered invalid.
func (t Ticket) MatchesRequest(req Request) bool {
	return t.AccessMethod == req.AccessMethod &&
		t.BasePath == req.BasePath &&
		t.UsernameOrID == req.UsernameOrID &&
		t.WorkspaceNameOrID == req.WorkspaceNameOrID &&
		t.AgentNameOrID == req.AgentNameOrID &&
		t.AppSlugOrPort == req.AppSlugOrPort
}

// GenerateTicket generates a workspace app ticket with the given key and
// payload. If the ticket doesn't have an expiry, it will be set to the current
// time plus the default expiry.
func GenerateTicket(key []byte, payload Ticket) (string, error) {
	if payload.Expiry.IsZero() {
		payload.Expiry = time.Now().Add(TicketExpiry)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", xerrors.Errorf("marshal payload to JSON: %w", err)
	}

	// We use symmetric signing with an RSA key to support satellites in the
	// future.
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: ticketSigningAlgorithm,
		Key:       key,
	}, nil)
	if err != nil {
		return "", xerrors.Errorf("create signer: %w", err)
	}

	signedObject, err := signer.Sign(payloadBytes)
	if err != nil {
		return "", xerrors.Errorf("sign payload: %w", err)
	}

	serialized, err := signedObject.CompactSerialize()
	if err != nil {
		return "", xerrors.Errorf("serialize JWS: %w", err)
	}

	return serialized, nil
}

// ParseTicket parses a workspace app ticket with the given key and returns the
// payload. If the ticket is invalid, an error is returned.
func ParseTicket(key []byte, ticketStr string) (Ticket, error) {
	object, err := jose.ParseSigned(ticketStr)
	if err != nil {
		return Ticket{}, xerrors.Errorf("parse JWS: %w", err)
	}
	if len(object.Signatures) != 1 {
		return Ticket{}, xerrors.New("expected 1 signature")
	}
	if object.Signatures[0].Header.Algorithm != string(ticketSigningAlgorithm) {
		return Ticket{}, xerrors.Errorf("expected ticket signing algorithm to be %q, got %q", ticketSigningAlgorithm, object.Signatures[0].Header.Algorithm)
	}

	output, err := object.Verify(key)
	if err != nil {
		return Ticket{}, xerrors.Errorf("verify JWS: %w", err)
	}

	var ticket Ticket
	err = json.Unmarshal(output, &ticket)
	if err != nil {
		return Ticket{}, xerrors.Errorf("unmarshal payload: %w", err)
	}
	if ticket.Expiry.Before(time.Now()) {
		return Ticket{}, xerrors.New("ticket expired")
	}

	return ticket, nil
}
