package coderd

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat project summaries
// @ID list-chat-project-summaries
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {array} codersdk.ChatProjectSummary
// @Router /api/experimental/chats/projects/summaries [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjectSummaries(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	rows, err := api.Database.GetChatProjectSummariesByUserID(ctx, database.GetChatProjectSummariesByUserIDParams{
		UserID:             apiKey.UserID,
		CountSharedMembers: api.chatProjectSharingAvailable(),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat project summaries.",
			Detail:  err.Error(),
		})
		return
	}
	compactChatProjectPinOrders(rows)
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(rows, func(row database.GetChatProjectSummariesByUserIDRow) codersdk.ChatProjectSummary {
		return db2sdk.ChatProjectSummary(row, api.convertChatProject(r, row.ChatProject))
	}))
}

// compactChatProjectPinOrders renumbers the pinned rows 1..N in their
// current order. The query numbers pins before authorization drops rows,
// such as a project whose grant lacks read, which would leave gaps.
func compactChatProjectPinOrders(rows []database.GetChatProjectSummariesByUserIDRow) {
	pinned := make([]*database.GetChatProjectSummariesByUserIDRow, 0, len(rows))
	for i := range rows {
		if rows[i].Pinned {
			pinned = append(pinned, &rows[i])
		}
	}
	slices.SortFunc(pinned, func(a, b *database.GetChatProjectSummariesByUserIDRow) int {
		return cmp.Compare(a.PinOrder, b.PinOrder)
	})
	for i, row := range pinned {
		row.PinOrder = int32(i + 1) //nolint:gosec // Bounded by the number of projects.
	}
}

// @Summary Pin chat project
// @ID pin-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.PinChatProjectRequest false "Pin chat project request"
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/pin [put]
// @x-apidocgen {"skip": true}
func (api *API) putChatProjectPin(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectPin(rw, r, project) {
		return
	}

	// The body is optional because pin_order is. Decoding reports io.EOF
	// only for an empty body, which a chunked request cannot announce in
	// its Content-Length.
	var req codersdk.PinChatProjectRequest
	err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, httpapi.DefaultMaxRequestBodyBytes)).Decode(&req)
	if err != nil && !errors.Is(err, io.EOF) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}
	var pinOrder sql.NullInt32
	if req.PinOrder != nil {
		if *req.PinOrder < 1 {
			writeChatProjectFieldError(ctx, rw, "pin_order", "Pin order must be at least 1.")
			return
		}
		pinOrder = sql.NullInt32{Int32: *req.PinOrder, Valid: true}
	}

	err = api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, chatProjectsOwnerLockID(apiKey.UserID)); err != nil {
			return xerrors.Errorf("lock chat project pins: %w", err)
		}
		if pinOrder.Valid {
			rows, err := tx.GetChatProjectSummariesByUserID(ctx, database.GetChatProjectSummariesByUserIDParams{
				UserID: apiKey.UserID,
			})
			if err != nil {
				return xerrors.Errorf("list visible chat project pins: %w", err)
			}
			pinOrder.Int32 = chatProjectPinPosition(rows, project.ID, pinOrder.Int32)
		}
		return tx.PinChatProject(ctx, database.PinChatProjectParams{
			UserID:    apiKey.UserID,
			ProjectID: project.ID,
			PinOrder:  pinOrder,
		})
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to pin chat project.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// chatProjectPinPosition translates a requested position among the pins
// the user sees into a position for PinChatProject. That query counts
// every pin its access predicate matches, but authorization can still
// hide some of them, such as a project whose grant lacks read or any
// granted project while chat sharing is disabled. A hidden pin between
// two visible ones would otherwise absorb the move.
// rows must be the authorized summaries: their pin orders number the
// matched pins, target included, in the order PinChatProject uses.
func chatProjectPinPosition(rows []database.GetChatProjectSummariesByUserIDRow, projectID uuid.UUID, requested int32) int32 {
	var targetOrder int32
	visible := make([]int32, 0, len(rows))
	for _, row := range rows {
		switch {
		case !row.Pinned:
		case row.ChatProject.ID == projectID:
			targetOrder = row.PinOrder
		default:
			visible = append(visible, row.PinOrder)
		}
	}
	if len(visible) == 0 {
		return 1
	}
	slices.Sort(visible)
	// PinChatProject numbers the other pins without the target.
	position := func(order int32) int32 {
		if targetOrder != 0 && targetOrder < order {
			return order - 1
		}
		return order
	}
	if int(requested) <= len(visible) {
		// Take the place of the visible pin now shown at that position.
		return position(visible[requested-1])
	}
	// Go right after the last visible pin.
	return position(visible[len(visible)-1]) + 1
}

// @Summary Unpin chat project
// @ID unpin-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/pin [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectPin(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectPin(rw, r, project) {
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, chatProjectsOwnerLockID(apiKey.UserID)); err != nil {
			return xerrors.Errorf("lock chat project pins: %w", err)
		}
		return tx.UnpinChatProject(ctx, database.UnpinChatProjectParams{
			UserID:    apiKey.UserID,
			ProjectID: project.ID,
		})
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to unpin chat project.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// chatProjectSharingAvailable reports whether summaries count the users a
// project is shared with. It matches the gate on the project ACL routes.
// Which projects are listed does not depend on it: as in GET
// /chats/projects, grant keys select candidates and RBAC decides.
func (api *API) chatProjectSharingAvailable() bool {
	return !api.chatSharingDisabled() && (buildinfo.IsDev() || api.Experiments.Enabled(codersdk.ExperimentChatProjectSharing))
}

// authorizeChatProjectPin writes a 404 and returns false unless the caller
// owns the project or holds a grant on it that RBAC honors.
// Summaries list only those projects, so role-based read access alone would
// leave the pin invisible.
func (api *API) authorizeChatProjectPin(rw http.ResponseWriter, r *http.Request, project database.ChatProject) bool {
	ctx := r.Context()
	userID := httpmw.APIKey(r).UserID
	// A pin is a personal preference, so reading the project is enough.
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) ||
		(project.OwnerID != userID && api.chatSharingDisabled()) {
		httpapi.ResourceNotFound(rw)
		return false
	}
	usable, err := chatd.ChatProjectUsableBy(ctx, api.Database, project, userID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error checking chat project access.",
			Detail:  err.Error(),
		})
		return false
	}
	if !usable {
		httpapi.ResourceNotFound(rw)
		return false
	}
	return true
}

// chatProjectsOwnerLockID serializes one user's project creates and pin
// writes. Pin writes renumber every pin of the user, so overlapping writes
// would otherwise deadlock or write positions onto rows another write just
// unpinned.
func chatProjectsOwnerLockID(userID uuid.UUID) int64 {
	return database.GenLockID("chat-projects-owner:" + userID.String())
}
