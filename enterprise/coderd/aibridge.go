package coderd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

const (
	maxListInterceptionsLimit     = 1000
	defaultListInterceptionsLimit = 100
)

// aiBridgeListInterceptions returns all AIBridge interceptions a user can read.
// Optional filters with query params
//
// @Summary List AIBridge interceptions
// @ID list-aibridge-interceptions
// @Security CoderSessionToken
// @Produce json
// @Tags AIBridge
// @Param q query string false "Search query in the format `key:value`. Available keys are: initiator, provider, model, started_after, started_before."
// @Param limit query int false "Page limit"
// @Param after_id query string false "Cursor pagination after ID"
// @Success 200 {object} codersdk.AIBridgeListInterceptionsResponse
// @Router /api/experimental/aibridge/interceptions [get]
func (api *API) aiBridgeListInterceptions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := coderd.ParsePagination(rw, r)
	if !ok {
		return
	}
	if page.Offset != 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Offset pagination is not supported.",
			Detail:  "Offset pagination is not supported for AIBridge interceptions. Use cursor pagination instead with after_id.",
		})
		return
	}
	if page.Limit == 0 {
		page.Limit = defaultListInterceptionsLimit
	}
	if page.Limit > maxListInterceptionsLimit || page.Limit < 1 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid pagination limit value.",
			Detail:  fmt.Sprintf("Pagination limit must be in range (0, %d]", maxListInterceptionsLimit),
		})
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.AIBridgeInterceptions(ctx, api.Database, queryStr, page, apiKey.UserID)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid workspace search query.",
			Validations: errs,
		})
		return
	}

	var rows []database.AIBridgeInterception
	err := api.Database.InTx(func(db database.Store) error {
		// Ensure the after_id interception exists and is visible to the user.
		if page.AfterID != uuid.Nil {
			_, err := db.GetAIBridgeInterceptionByID(ctx, page.AfterID)
			if err != nil {
				return xerrors.Errorf("get aibridge interception by id %s for cursor pagination: %w", page.AfterID, err)
			}
		}

		var err error
		// This only returns authorized interceptions (when using dbauthz).
		rows, err = db.ListAIBridgeInterceptions(ctx, filter)
		if err != nil {
			return xerrors.Errorf("list aibridge interceptions: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting AIBridge interceptions.",
			Detail:  err.Error(),
		})
		return
	}

	// This fetches the other rows associated with the interceptions.
	items, err := populatedAndConvertAIBridgeInterceptions(ctx, api.Database, rows)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting database rows to API response.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AIBridgeListInterceptionsResponse{
		Results: items,
	})
}

func populatedAndConvertAIBridgeInterceptions(ctx context.Context, db database.Store, dbInterceptions []database.AIBridgeInterception) ([]codersdk.AIBridgeInterception, error) {
	ids := make([]uuid.UUID, len(dbInterceptions))
	for i, row := range dbInterceptions {
		ids[i] = row.ID
	}

	//nolint:gocritic // This is a system function until we implement a join for aibridge interceptions. AIBridge interception subresources use the same authorization call as their parent.
	tokenUsagesRows, err := db.ListAIBridgeTokenUsagesByInterceptionIDs(dbauthz.AsSystemRestricted(ctx), ids)
	if err != nil {
		return nil, xerrors.Errorf("get linked aibridge token usages from database: %w", err)
	}
	tokenUsagesMap := make(map[uuid.UUID][]database.AIBridgeTokenUsage, len(dbInterceptions))
	for _, row := range tokenUsagesRows {
		tokenUsagesMap[row.InterceptionID] = append(tokenUsagesMap[row.InterceptionID], row)
	}

	//nolint:gocritic // This is a system function until we implement a join for aibridge interceptions. AIBridge interception subresources use the same authorization call as their parent.
	userPromptRows, err := db.ListAIBridgeUserPromptsByInterceptionIDs(dbauthz.AsSystemRestricted(ctx), ids)
	if err != nil {
		return nil, xerrors.Errorf("get linked aibridge user prompts from database: %w", err)
	}
	userPromptsMap := make(map[uuid.UUID][]database.AIBridgeUserPrompt, len(dbInterceptions))
	for _, row := range userPromptRows {
		userPromptsMap[row.InterceptionID] = append(userPromptsMap[row.InterceptionID], row)
	}

	//nolint:gocritic // This is a system function until we implement a join for aibridge interceptions. AIBridge interception subresources use the same authorization call as their parent.
	toolUsagesRows, err := db.ListAIBridgeToolUsagesByInterceptionIDs(dbauthz.AsSystemRestricted(ctx), ids)
	if err != nil {
		return nil, xerrors.Errorf("get linked aibridge tool usages from database: %w", err)
	}
	toolUsagesMap := make(map[uuid.UUID][]database.AIBridgeToolUsage, len(dbInterceptions))
	for _, row := range toolUsagesRows {
		toolUsagesMap[row.InterceptionID] = append(toolUsagesMap[row.InterceptionID], row)
	}

	items := make([]codersdk.AIBridgeInterception, len(dbInterceptions))
	for i, row := range dbInterceptions {
		items[i] = db2sdk.AIBridgeInterception(row, tokenUsagesMap[row.ID], userPromptsMap[row.ID], toolUsagesMap[row.ID])
	}

	return items, nil
}
