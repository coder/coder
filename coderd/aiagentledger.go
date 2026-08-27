package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// maxAIAgentJournalEntries caps each journal read. One more than this is
	// requested, so that reaching the cap is detectable and the response can
	// say so rather than presenting a short answer as a complete one.
	maxAIAgentJournalEntries = 500

	maxAIAgentNetworkEventsPageSize = 100
)

// journalRank orders entries that share an effective date.
//
// Creating an AI agent writes three entries in one transaction, and Postgres
// now() is transaction start time, so all three carry an identical recording
// date and nothing about the clock separates them. The order is the model's
// rather than the clock's: an entity that does not exist cannot be party to an
// agency relation, so the agent precedes the grant; and a credential confers a
// means of acting on a party that already holds the authority to act, so the
// grant precedes the credential.
//
// The same order reads correctly at a retirement, where both lapses follow the
// retirement that entailed them. It must not be read as one causing the next:
// consequences of a common cause are siblings, and neither entails the other.
func journalRank(j codersdk.AIAgentJournal) int {
	switch j {
	case codersdk.AIAgentJournalAIAgent:
		return 0
	case codersdk.AIAgentJournalAuthorization:
		return 1
	case codersdk.AIAgentJournalCredential:
		return 2
	default:
		return 3
	}
}

// requestingUser refuses a requester that is not a user.
//
// These routes stand in user typed slots. An AI agent authorizes as its owner,
// so a scope check alone would let one read them with its sponsor's
// permissions. Refusing by kind does not depend on getting an allow list right.
func requestingUser(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, ok := httpmw.APIKey(r).UserID()
	if !ok {
		httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
			Message: "Only a user may read an AI agent's record.",
			Detail:  "The credential presented is held by something other than a user.",
		})
		return uuid.Nil, false
	}
	return userID, true
}

// aiAgentForRead resolves the {aiagent} path parameter and establishes that the
// requester may read that agent's record.
//
// The ledger row is read under a system context because every journal and
// ledger read is guarded by ResourceSystem, which no ordinary user holds. The
// caller is then authorized against the agent's owner, which is the guard the
// existing AI agents endpoint applies to GetAIAgentsByOwner.
//
// Proof of concept cheat: authorization is coarse. An AI agent has no RBAC
// resource of its own, and giving it one reaches policy.rego.
func (api *API) aiAgentForRead(rw http.ResponseWriter, r *http.Request) (database.AIAgentLedger, bool) {
	ctx := r.Context()

	if _, ok := requestingUser(rw, r); !ok {
		return database.AIAgentLedger{}, false
	}
	agentID, ok := httpmw.ParseUUIDParam(rw, r, "aiagent")
	if !ok {
		return database.AIAgentLedger{}, false
	}

	// nolint:gocritic // Guarded by ResourceSystem; the caller is authorized
	// against the agent's owner immediately below.
	row, err := api.Database.GetAIAgentLedgerRowByID(dbauthz.AsSystemRestricted(ctx), agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return database.AIAgentLedger{}, false
		}
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI agent ledger row: %w", err))
		return database.AIAgentLedger{}, false
	}

	// An owner is a pair, and only a user owner can be authorized against a
	// user object. A system actor owning an agent is the prebuilt case, which
	// nothing here can express, so it is refused rather than guessed at.
	if row.OwnerType != string(entity.TypeUser) {
		httpapi.ResourceNotFound(rw)
		return database.AIAgentLedger{}, false
	}
	if !api.Authorize(r, policy.ActionReadPersonal, rbac.ResourceUserObject(row.OwnerID)) {
		httpapi.ResourceNotFound(rw)
		return database.AIAgentLedger{}, false
	}
	return row, true
}

func convertAIAgentLedgerRow(row database.AIAgentLedger) codersdk.AIAgentLedgerRow {
	return codersdk.AIAgentLedgerRow{
		ID: row.ID,
		// Computed rather than stored, from the identifier and the creation
		// site, so nothing persists it and nothing has to keep it in step.
		DisplayName:      entity.DisplayName(entity.CreationSiteType(row.CreationSiteType), row.ID),
		OwnerType:        row.OwnerType,
		OwnerID:          row.OwnerID,
		State:            codersdk.AIAgentState(row.State),
		CreationSiteType: codersdk.AIAgentCreationSiteType(row.CreationSiteType),
		CreationSiteID:   row.CreationSiteID,
		CreationTime:     row.CreationTime,
	}
}

// @Summary List the AI agents in the ledger owned by the requester
// @ID get-ai-agent-ledger
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Success 200 {array} codersdk.AIAgentLedgerRow
// @Router /api/v2/aiagents/ledger [get]
// @x-apidocgen {"skip": true}
func (api *API) aiAgentLedger(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := requestingUser(rw, r)
	if !ok {
		return
	}

	rows, err := api.Database.GetAIAgentsByOwner(ctx, userID)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI agents by owner: %w", err))
		return
	}

	// The ledger keeps retired rows, so this is the whole population and not
	// only the live part of it.
	response := make([]codersdk.AIAgentLedgerRow, 0, len(rows))
	for _, row := range rows {
		response = append(response, convertAIAgentLedgerRow(row))
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Get one AI agent's ledger row
// @ID get-ai-agent-ledger-row
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param aiagent path string true "AI agent ID" format(uuid)
// @Success 200 {object} codersdk.AIAgentLedgerRow
// @Router /api/v2/aiagents/{aiagent}/ledger [get]
// @x-apidocgen {"skip": true}
func (api *API) aiAgentLedgerRow(rw http.ResponseWriter, r *http.Request) {
	row, ok := api.aiAgentForRead(rw, r)
	if !ok {
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, convertAIAgentLedgerRow(row))
}

// @Summary List one AI agent's lifecycle journal entries
// @ID get-ai-agent-lifecycle-journals
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param aiagent path string true "AI agent ID" format(uuid)
// @Success 200 {object} codersdk.AIAgentLifecycleJournalsResponse
// @Router /api/v2/aiagents/{aiagent}/lifecycle-journals [get]
// @x-apidocgen {"skip": true}
func (api *API) aiAgentLifecycleJournals(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent, ok := api.aiAgentForRead(rw, r)
	if !ok {
		return
	}

	// nolint:gocritic // Journals are guarded by ResourceSystem. The caller was
	// authorized against the agent's owner in aiAgentForRead.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	entries, truncated, err := api.readAIAgentJournals(sysCtx, agent.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Sorted here rather than by the reader, because the rank that breaks a tie
	// is a fact about the model and belongs where the model is known.
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].EffectiveDate.Equal(entries[j].EffectiveDate) {
			return entries[i].EffectiveDate.Before(entries[j].EffectiveDate)
		}
		if ri, rj := journalRank(entries[i].Journal), journalRank(entries[j].Journal); ri != rj {
			return ri < rj
		}
		if entries[i].EntryID != entries[j].EntryID {
			return entries[i].EntryID < entries[j].EntryID
		}
		return entries[i].Line < entries[j].Line
	})

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AIAgentLifecycleJournalsResponse{
		Entries:   entries,
		Truncated: truncated,
	})
}

// readAIAgentJournals gathers the entries of all three lifecycle journals for
// one agent.
//
// Only the AI agent journal has the agent as its subject. An authorization's
// subject is the authorization and a credential's is the credential, so each is
// reached by first asking which of them this agent holds. The traversals are
// N+1 by row, which is a proof of concept cheat.
func (api *API) readAIAgentJournals(ctx context.Context, agentID uuid.UUID) ([]codersdk.AIAgentLifecycleJournalEntry, bool, error) {
	var (
		entries   []codersdk.AIAgentLifecycleJournalEntry
		truncated bool
	)

	agentRows, err := api.Database.GetAIAgentLifecycleEntriesBySubject(ctx, database.GetAIAgentLifecycleEntriesBySubjectParams{
		Subject: agentID,
		Limit:   maxAIAgentJournalEntries + 1,
	})
	if err != nil {
		return nil, false, xerrors.Errorf("get AI agent lifecycle entries: %w", err)
	}
	if len(agentRows) > maxAIAgentJournalEntries {
		agentRows, truncated = agentRows[:maxAIAgentJournalEntries], true
	}
	for _, row := range agentRows {
		actorID := row.Actor
		entries = append(entries, codersdk.AIAgentLifecycleJournalEntry{
			Journal: codersdk.AIAgentJournalAIAgent,
			EntryID: row.EntryID,
			// The AI agent journal is normalized and its entry carries the
			// event and subject directly, so every entry is line zero.
			Line:          0,
			Event:         row.Event,
			Subject:       row.Subject,
			EffectiveDate: row.EffectiveDate,
			RecordingDate: row.RecordingDate,
			ActorType:     row.ActorType,
			ActorID:       &actorID,
		})
	}

	authorizations, err := api.Database.GetAuthorizationLedgerRowsByAgent(ctx, database.GetAuthorizationLedgerRowsByAgentParams{
		AgentType: string(entity.TypeAIAgent),
		AgentID:   agentID,
	})
	if err != nil {
		return nil, false, xerrors.Errorf("get authorizations by agent: %w", err)
	}
	for _, authorization := range authorizations {
		rows, err := api.Database.GetAuthorizationLifecycleJournalLinesBySubject(ctx, database.GetAuthorizationLifecycleJournalLinesBySubjectParams{
			Subject: authorization.ID,
			Limit:   maxAIAgentJournalEntries + 1,
		})
		if err != nil {
			return nil, false, xerrors.Errorf("get authorization lifecycle entries: %w", err)
		}
		if len(rows) > maxAIAgentJournalEntries {
			rows, truncated = rows[:maxAIAgentJournalEntries], true
		}
		for _, row := range rows {
			entry := codersdk.AIAgentLifecycleJournalEntry{
				Journal: codersdk.AIAgentJournalAuthorization,
				EntryID: row.EntryID,
				Line:    int32(row.Line),
				Event:   row.Event,
				Subject: row.Subject,
			}
			// Supplied by the query's join onto line zero, so they are present
			// whichever line this subject sits on.
			if row.EffectiveDate.Valid {
				entry.EffectiveDate = row.EffectiveDate.Time
			}
			if row.RecordingDate.Valid {
				entry.RecordingDate = row.RecordingDate.Time
			}
			if row.ActorType.Valid && row.Actor.Valid {
				actorID := row.Actor.UUID
				entry.ActorType, entry.ActorID = row.ActorType.String, &actorID
			}
			entries = append(entries, entry)
		}
	}

	credentials, err := api.Database.GetCredentialLedgerRowsByHolder(ctx, database.GetCredentialLedgerRowsByHolderParams{
		HolderType: string(entity.TypeAIAgent),
		HolderID:   agentID,
	})
	if err != nil {
		return nil, false, xerrors.Errorf("get credentials by holder: %w", err)
	}
	for _, credential := range credentials {
		rows, err := api.Database.GetCredentialLifecycleJournalEntriesBySubject(ctx, database.GetCredentialLifecycleJournalEntriesBySubjectParams{
			Subject: credential.ID,
			Limit:   maxAIAgentJournalEntries + 1,
		})
		if err != nil {
			return nil, false, xerrors.Errorf("get credential lifecycle entries: %w", err)
		}
		if len(rows) > maxAIAgentJournalEntries {
			rows, truncated = rows[:maxAIAgentJournalEntries], true
		}
		for _, row := range rows {
			entry := codersdk.AIAgentLifecycleJournalEntry{
				Journal:       codersdk.AIAgentJournalCredential,
				EntryID:       row.EntryID,
				Line:          int32(row.Line),
				Event:         row.Event,
				Subject:       row.Subject,
				EffectiveDate: row.EffectiveDate,
				RecordingDate: row.RecordingDate,
			}
			// Absent exactly when the operation was entailed. Never unknown.
			if row.ActorType.Valid && row.Actor.Valid {
				actorID := row.Actor.UUID
				entry.ActorType = row.ActorType.String
				entry.ActorID = &actorID
			}
			entries = append(entries, entry)
		}
	}

	return entries, truncated, nil
}

// @Summary List one AI agent's sandbox confinement sessions
// @ID get-ai-agent-sandbox-sessions-log
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param aiagent path string true "AI agent ID" format(uuid)
// @Success 200 {array} codersdk.AISandboxSession
// @Router /api/v2/aiagents/{aiagent}/sandbox-sessions-log [get]
// @x-apidocgen {"skip": true}
func (api *API) aiAgentSandboxSessionsLog(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent, ok := api.aiAgentForRead(rw, r)
	if !ok {
		return
	}

	// nolint:gocritic // Guarded by ResourceSystem. The caller was authorized
	// against the agent's owner in aiAgentForRead.
	sessions, err := api.Database.GetAISandboxSessionsByAIAgentID(dbauthz.AsSystemRestricted(ctx), agent.ID)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandbox sessions: %w", err))
		return
	}

	response := make([]codersdk.AISandboxSession, 0, len(sessions))
	for _, session := range sessions {
		response = append(response, convertAISandboxSession(session))
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary List one AI agent's sandbox egress decisions
// @ID get-ai-agent-network-events-log
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param aiagent path string true "AI agent ID" format(uuid)
// @Param after_id query int false "Return events with database ID greater than after_id"
// @Param limit query int false "Page size, 1 to 100. Defaults to 100."
// @Success 200 {array} codersdk.AISandboxNetworkEventView
// @Router /api/v2/aiagents/{aiagent}/network-events-log [get]
// @x-apidocgen {"skip": true}
func (api *API) aiAgentNetworkEventsLog(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent, ok := api.aiAgentForRead(rw, r)
	if !ok {
		return
	}

	parser := httpapi.NewQueryParamParser()
	afterID := parser.PositiveInt64(r.URL.Query(), 0, "after_id")
	limit := parser.PositiveInt32(r.URL.Query(), maxAIAgentNetworkEventsPageSize, "limit")
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}
	if limit < 1 || limit > maxAIAgentNetworkEventsPageSize {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid limit parameter (1-%d).", maxAIAgentNetworkEventsPageSize),
		})
		return
	}

	// Filters on the event's own attribution snapshot rather than joining
	// through its session, which is what lets an event outlive the session.
	// nolint:gocritic // Guarded by ResourceSystem. The caller was authorized
	// against the agent's owner in aiAgentForRead.
	events, err := api.Database.GetAISandboxNetworkEventsByAIAgentIDPaged(dbauthz.AsSystemRestricted(ctx), database.GetAISandboxNetworkEventsByAIAgentIDPagedParams{
		AIAgentID:  agent.ID,
		AfterID:    afterID,
		LimitCount: limit,
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandbox network events: %w", err))
		return
	}

	response := make([]codersdk.AISandboxNetworkEventView, 0, len(events))
	for _, event := range events {
		response = append(response, codersdk.AISandboxNetworkEventView{
			ID:             event.ID,
			SessionID:      event.SessionID,
			OccurredAt:     event.OccurredAt,
			Protocol:       codersdk.AISandboxNetworkProtocol(event.Protocol),
			Host:           event.Host,
			Port:           int(event.Port),
			Action:         codersdk.AISandboxNetworkEventAction(event.Action),
			PolicyRevision: event.PolicyRevision,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}
