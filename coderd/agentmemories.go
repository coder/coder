package coderd

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/agentmemory"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

const (
	agentMemoryPageSize             = 25
	agentMemoryJSONEscapeExpansion  = 6
	agentMemoryRequestEnvelopeBytes = 1024
)

var errAgentMemoryStale = xerrors.New("agent memory changed since it was loaded")

// @Summary List agent memory children
// @ID list-agent-memory-children
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param directory query string false "Canonical virtual directory" default(/)
// @Param offset query int false "Zero-based offset" default(0)
// @Success 200 {object} codersdk.AgentMemoryChildrenResponse
// @Router /api/experimental/users/{user}/agent-memories [get]
// @x-apidocgen {"skip": true}
func (api *API) agentMemoryChildren(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	directory := r.URL.Query().Get("directory")
	if directory == "" {
		directory = "/"
	}
	if err := agentmemory.ValidateDirectory(directory); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid memory directory.", Detail: err.Error()})
		return
	}
	offset, err := parseAgentMemoryOffset(r.URL.Query().Get("offset"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid offset.", Detail: err.Error()})
		return
	}
	rows, err := api.Database.ListAgentMemoryChildren(ctx, database.ListAgentMemoryChildrenParams{
		UserID: user.ID, Directory: directory, OffsetValue: offset,
	})
	if err != nil {
		writeAgentMemoryDatabaseError(rw, err)
		return
	}
	hasMore := len(rows) > agentMemoryPageSize
	if hasMore {
		rows = rows[:agentMemoryPageSize]
	}
	response := codersdk.AgentMemoryChildrenResponse{Entries: db2sdk.AgentMemoryEntries(rows)}
	if hasMore {
		response.NextOffset = new(offset + agentMemoryPageSize)
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

func parseAgentMemoryOffset(value string) (int32, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		return 0, xerrors.New("offset must be a non-negative 32-bit integer")
	}
	return int32(parsed), nil
}

// @Summary Get default agent memory
// @ID get-default-agent-memory
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Success 200 {object} codersdk.AgentMemory
// @Router /api/experimental/users/{user}/agent-memories/default [get]
// @x-apidocgen {"skip": true}
func (api *API) defaultAgentMemory(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	memory, err := api.Database.GetDefaultAgentMemoryByUserID(ctx, httpmw.UserParam(r).ID)
	if err != nil {
		writeAgentMemoryDatabaseError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AgentMemory(memory))
}

func agentMemoryID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return httpmw.ParseUUIDParam(rw, r, "memoryID")
}

// @Summary Get agent memory
// @ID get-agent-memory
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param memoryID path string true "Memory UUID"
// @Success 200 {object} codersdk.AgentMemory
// @Router /api/experimental/users/{user}/agent-memories/{memoryID} [get]
// @x-apidocgen {"skip": true}
func (api *API) agentMemoryByID(rw http.ResponseWriter, r *http.Request) {
	id, ok := agentMemoryID(rw, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := httpmw.UserParam(r)
	memory, err := api.Database.GetAgentMemoryByUserIDAndID(ctx, database.GetAgentMemoryByUserIDAndIDParams{UserID: user.ID, ID: id})
	if err != nil {
		writeAgentMemoryDatabaseError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AgentMemory(memory))
}

// @Summary Update agent memory
// @ID update-agent-memory
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param memoryID path string true "Memory UUID"
// @Param request body codersdk.UpdateAgentMemoryRequest true "Updated memory content"
// @Success 200 {object} codersdk.AgentMemory
// @Router /api/experimental/users/{user}/agent-memories/{memoryID} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchAgentMemory(rw http.ResponseWriter, r *http.Request) {
	id, ok := agentMemoryID(rw, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := httpmw.UserParam(r)
	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.AgentMemory](rw, &audit.RequestParams{
		Audit: *auditor, Log: api.Logger, Request: r, Action: database.AuditActionWrite,
	})
	defer commitAudit()

	r.Body = http.MaxBytesReader(rw, r.Body, agentmemory.MaxContentBytes*agentMemoryJSONEscapeExpansion+agentMemoryRequestEnvelopeBytes)
	var req codersdk.UpdateAgentMemoryRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if len(req.Content) > agentmemory.MaxContentBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Memory content is too large.", Detail: "Memory content must not exceed 65536 bytes."})
		return
	}

	var oldMemory, updated database.AgentMemory
	err := api.Database.InTx(func(tx database.Store) error {
		memory, err := tx.GetAgentMemoryByUserIDAndIDForUpdate(ctx, database.GetAgentMemoryByUserIDAndIDForUpdateParams{UserID: user.ID, ID: id})
		if err != nil {
			return err
		}
		if !memory.UpdatedAt.Equal(req.ExpectedUpdatedAt) {
			return errAgentMemoryStale
		}
		oldMemory = memory
		updated, err = tx.UpdateAgentMemoryContent(ctx, database.UpdateAgentMemoryContentParams{UserID: user.ID, Path: memory.Path, Content: req.Content})
		return err
	}, nil)
	if errors.Is(err, errAgentMemoryStale) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "Memory changed while you were editing it.", Detail: "Reload the latest memory before saving again."})
		return
	}
	if err != nil {
		writeAgentMemoryDatabaseError(rw, err)
		return
	}
	aReq.Old = oldMemory
	aReq.New = updated
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AgentMemory(updated))
}

// @Summary Delete agent memory
// @ID delete-agent-memory
// @Security CoderSessionToken
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param memoryID path string true "Memory UUID"
// @Success 204
// @Router /api/experimental/users/{user}/agent-memories/{memoryID} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteAgentMemory(rw http.ResponseWriter, r *http.Request) {
	id, ok := agentMemoryID(rw, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := httpmw.UserParam(r)
	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.AgentMemory](rw, &audit.RequestParams{
		Audit: *auditor, Log: api.Logger, Request: r, Action: database.AuditActionDelete,
	})
	defer commitAudit()
	deleted, err := api.Database.DeleteAgentMemoryByUserIDAndID(ctx, database.DeleteAgentMemoryByUserIDAndIDParams{UserID: user.ID, ID: id})
	if err != nil {
		writeAgentMemoryDatabaseError(rw, err)
		return
	}
	aReq.Old = deleted
	rw.WriteHeader(http.StatusNoContent)
}

func writeAgentMemoryDatabaseError(rw http.ResponseWriter, err error) {
	if httpapi.IsUnauthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.InternalServerError(rw, err)
}
