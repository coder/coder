package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// AIAgentState is the state an AI agent's ledger row holds. The ledger keeps
// retired rows rather than deleting them, so a listing shows the whole
// population and not only the live part of it.
//
// dormant is reserved and unreachable in the machine the proof of concept
// implements, which has active and retired only. Code switching exhaustively
// over these values has to handle a state that cannot occur.
type AIAgentState string

const (
	AIAgentStateActive  AIAgentState = "active"
	AIAgentStateDormant AIAgentState = "dormant"
	AIAgentStateRetired AIAgentState = "retired"
)

// AIAgentCreationSiteType is what kind of thing an AI agent was created in.
//
// It is a fact about the creation event and not a statement of where the agent
// now is. An agent that moved would keep the site it was created in, and
// nothing moves one today.
type AIAgentCreationSiteType string

const (
	AIAgentCreationSiteTypeChat      AIAgentCreationSiteType = "chat"
	AIAgentCreationSiteTypeWorkspace AIAgentCreationSiteType = "workspace"
)

// AIAgentLedgerRow is what the ledger currently holds about one AI agent.
//
// It speaks the ledger's own vocabulary. The older AIAgent type reports the
// same agent with an origin and a deleted flag, which are the words its public
// surface already used.
//
// DisplayName is computed from the identifier and the creation site rather than
// stored, so nothing persists it and nothing has to keep it in step.
type AIAgentLedgerRow struct {
	ID               uuid.UUID               `json:"id" format:"uuid"`
	DisplayName      string                  `json:"display_name"`
	OwnerType        string                  `json:"owner_type"`
	OwnerID          uuid.UUID               `json:"owner_id" format:"uuid"`
	State            AIAgentState            `json:"state" enums:"active,dormant,retired"`
	CreationSiteType AIAgentCreationSiteType `json:"creation_site_type" enums:"chat,workspace"`
	CreationSiteID   uuid.UUID               `json:"creation_site_id" format:"uuid"`
	CreationTime     time.Time               `json:"creation_time" format:"date-time"`
}

// AIAgentJournal names which lifecycle journal an entry was read from. One
// journal records one entity's model, so the name also says what the entry's
// subject identifies.
type AIAgentJournal string

const (
	AIAgentJournalAIAgent       AIAgentJournal = "ai_agent"
	AIAgentJournalAuthorization AIAgentJournal = "authorization"
	AIAgentJournalCredential    AIAgentJournal = "credential"
)

// AIAgentLifecycleJournalEntry is one line of one entry, from one of the three
// lifecycle journals, flattened into a shape a reader can put in order.
//
// Subject identifies whatever the journal named by Journal is about: the agent
// itself, one of its authorizations, or one of its credentials.
//
// Entries sharing an EntryID are one event and not several that coincide. A
// retirement ends every authorization and every credential a holder has as a
// single event, so it arrives as one entry with a line apiece.
type AIAgentLifecycleJournalEntry struct {
	Journal AIAgentJournal `json:"journal" enums:"ai_agent,authorization,credential"`
	EntryID int64          `json:"entry_id"`
	Line    int32          `json:"line"`
	Event   string         `json:"event"`
	Subject uuid.UUID      `json:"subject" format:"uuid"`
	// EffectiveDate is when the event occurred and RecordingDate when the entry
	// was made. They differ whenever an entry was written after the fact, and a
	// reader ordering events wants the first.
	EffectiveDate time.Time `json:"effective_date" format:"date-time"`
	RecordingDate time.Time `json:"recording_date" format:"date-time"`
	// ActorType and ActorID are absent exactly when the operation was entailed,
	// which is to say it followed by necessity from something already recorded
	// and nobody performed it. Their absence never means the actor is unknown.
	ActorType string     `json:"actor_type,omitempty"`
	ActorID   *uuid.UUID `json:"actor_id,omitempty" format:"uuid"`
}

// AIAgentLifecycleJournalsResponse carries the entries of all three lifecycle
// journals for one agent.
//
// Truncated says a per journal cap was reached and entries were left unread.
// It exists so that a short answer is never mistaken for a complete one.
type AIAgentLifecycleJournalsResponse struct {
	Entries   []AIAgentLifecycleJournalEntry `json:"entries"`
	Truncated bool                           `json:"truncated"`
}

// AIAgentLedger lists the AI agents the requesting user owns.
//
// The ledger keeps retired rows, so this is the whole population and not only
// the live part of it.
func (c *Client) AIAgentLedger(ctx context.Context) ([]AIAgentLedgerRow, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aiagents/ledger", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var rows []AIAgentLedgerRow
	return rows, ReadBodyAsJSON(res, &rows)
}

// AIAgentLedgerRowByID reads what the ledger currently holds about one agent.
func (c *Client) AIAgentLedgerRowByID(ctx context.Context, id uuid.UUID) (AIAgentLedgerRow, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aiagents/%s/ledger", id), nil)
	if err != nil {
		return AIAgentLedgerRow{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIAgentLedgerRow{}, ReadBodyAsError(res)
	}
	var row AIAgentLedgerRow
	return row, ReadBodyAsJSON(res, &row)
}

// AIAgentLifecycleJournals reads one agent's entries from the three lifecycle
// journals, in the order the model puts them.
func (c *Client) AIAgentLifecycleJournals(ctx context.Context, id uuid.UUID) (AIAgentLifecycleJournalsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aiagents/%s/lifecycle-journals", id), nil)
	if err != nil {
		return AIAgentLifecycleJournalsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIAgentLifecycleJournalsResponse{}, ReadBodyAsError(res)
	}
	var response AIAgentLifecycleJournalsResponse
	return response, ReadBodyAsJSON(res, &response)
}

// AIAgentSandboxSessionsLog reads the confinement sessions naming one agent.
func (c *Client) AIAgentSandboxSessionsLog(ctx context.Context, id uuid.UUID) ([]AISandboxSession, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aiagents/%s/sandbox-sessions-log", id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var sessions []AISandboxSession
	return sessions, ReadBodyAsJSON(res, &sessions)
}

// AIAgentNetworkEventsLog reads the egress decisions naming one agent, oldest
// first, paged by row identifier.
func (c *Client) AIAgentNetworkEventsLog(ctx context.Context, id uuid.UUID, afterID int64, limit int32) ([]AISandboxNetworkEventView, error) {
	path := fmt.Sprintf("/api/v2/aiagents/%s/network-events-log?after_id=%d", id, afterID)
	if limit > 0 {
		path = fmt.Sprintf("%s&limit=%d", path, limit)
	}
	res, err := c.Request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var events []AISandboxNetworkEventView
	return events, ReadBodyAsJSON(res, &events)
}
