package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get group AI budget
// @ID get-group-ai-budget
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Success 200 {object} codersdk.GroupAIBudget
// @Router /api/v2/groups/{group}/ai/budget [get]
func (api *API) groupAIBudget(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	group := httpmw.GroupParam(r)

	budget, err := api.Database.GetGroupAIBudget(ctx, group.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, groupAIBudgetToSDK(budget))
}

// @Summary Upsert group AI budget
// @ID upsert-group-ai-budget
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Param request body codersdk.UpsertGroupAIBudgetRequest true "Upsert group AI budget request"
// @Success 200 {object} codersdk.GroupAIBudget
// @Router /api/v2/groups/{group}/ai/budget [put]
func (api *API) upsertGroupAIBudget(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	group := httpmw.GroupParam(r)

	var req codersdk.UpsertGroupAIBudgetRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.SpendLimitMicros <= 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "spend_limit_micros must be greater than zero.",
			Validations: []codersdk.ValidationError{{
				Field:  "spend_limit_micros",
				Detail: "must be greater than zero",
			}},
		})
		return
	}

	budget, err := api.Database.UpsertGroupAIBudget(ctx, database.UpsertGroupAIBudgetParams{
		GroupID:    group.ID,
		SpendLimit: req.SpendLimitMicros,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, groupAIBudgetToSDK(budget))
}

// @Summary Delete group AI budget
// @ID delete-group-ai-budget
// @Security CoderSessionToken
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Success 204
// @Router /api/v2/groups/{group}/ai/budget [delete]
func (api *API) deleteGroupAIBudget(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	group := httpmw.GroupParam(r)

	if _, err := api.Database.DeleteGroupAIBudget(ctx, group.ID); err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func groupAIBudgetToSDK(b database.GroupAiBudget) codersdk.GroupAIBudget {
	return codersdk.GroupAIBudget{
		GroupID:          b.GroupID,
		SpendLimitMicros: b.SpendLimit,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
	}
}
