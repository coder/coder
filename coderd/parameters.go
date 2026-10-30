package coderd

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

// @Summary Evaluate dynamic parameters for template version
// @Description Evaluates the parameters of a template version once for the given `inputs` and returns the resulting parameters and any diagnostics.
// @Description Use it to render the workspace creation form, or open `GET /api/v2/templateversions/{templateversion}/dynamic-parameters` for a live session.
// @Description The call returns `425` until the template version's import job has finished.
// @ID evaluate-dynamic-parameters-for-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Accept json
// @Produce json
// @Param request body codersdk.DynamicParametersRequest true "Parameter values to evaluate the template version with, as `inputs`."
// @Success 200 {object} codersdk.DynamicParametersResponse
// @Router /api/v2/templateversions/{templateversion}/dynamic-parameters/evaluate [post]
func (api *API) templateVersionDynamicParametersEvaluate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.DynamicParametersRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	api.templateVersionDynamicParameters(false, req)(rw, r)
}

// @Summary Open dynamic parameters WebSocket by template version
// @Description Opens a WebSocket that re-evaluates a template version's parameters as the user fills in the workspace creation form.
// @Description After the upgrade, the server sends a JSON message with `id` set to `-1`, `parameters`, and `diagnostics`, computed from no inputs.
// @Description Send a JSON message with an `id`, the current `inputs`, and optionally an `owner_id`, and the server replies with a message that has the same `id` and the re-evaluated `parameters` and `diagnostics`.
// @Description An omitted `owner_id` keeps the previous owner, which starts as you or as the user in the `user_id` query parameter.
// @Description The server closes the connection after 30 minutes.
// @ID open-dynamic-parameters-websocket-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Param user_id query string false "ID of the user, or `me`, who owns the workspace that the first evaluation is for. The default is you."
// @Success 101
// @Router /api/v2/templateversions/{templateversion}/dynamic-parameters [get]
func (api *API) templateVersionDynamicParametersWebsocket(rw http.ResponseWriter, r *http.Request) {
	apikey := httpmw.APIKey(r)
	userID := apikey.UserID

	qUserID := r.URL.Query().Get("user_id")
	if qUserID != "" && qUserID != codersdk.Me {
		uid, err := uuid.Parse(qUserID)
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid user_id query parameter",
				Detail:  err.Error(),
			})
			return
		}
		userID = uid
	}

	api.templateVersionDynamicParameters(true, codersdk.DynamicParametersRequest{
		ID:      -1,
		Inputs:  map[string]string{},
		OwnerID: userID,
	})(rw, r)
}

// The `listen` control flag determines whether to open a websocket connection to
// handle the request or not. This same function is used to 'evaluate' a template
// as a single invocation, or to 'listen' for a back and forth interaction with
// the user to update the form as they type.
//
//nolint:revive // listen is a control flag
func (api *API) templateVersionDynamicParameters(listen bool, initial codersdk.DynamicParametersRequest) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		templateVersion := httpmw.TemplateVersionParam(r)

		renderer, err := dynamicparameters.Prepare(ctx, api.Database, api.FileCache, templateVersion.ID,
			dynamicparameters.WithTemplateVersion(templateVersion),
			dynamicparameters.WithPreviewOptions(dynamicparameters.PreviewOptions(api.DeploymentValues)...),
		)
		if err != nil {
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}

			if xerrors.Is(err, dynamicparameters.ErrTemplateVersionNotReady) {
				httpapi.Write(ctx, rw, http.StatusTooEarly, codersdk.Response{
					Message: "Template version job has not finished",
				})
				return
			}

			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching template version data.",
				Detail:  err.Error(),
			})
			return
		}
		defer renderer.Close()

		if listen {
			api.handleParameterWebsocket(rw, r, initial, renderer)
		} else {
			api.handleParameterEvaluate(rw, r, initial, renderer)
		}
	}
}

func (*API) handleParameterEvaluate(rw http.ResponseWriter, r *http.Request, initial codersdk.DynamicParametersRequest, render dynamicparameters.Renderer) {
	ctx := r.Context()

	// Send an initial form state, computed without any user input.
	result, diagnostics := render.Render(ctx, initial.OwnerID, initial.Inputs)
	response := codersdk.DynamicParametersResponse{
		ID:          0,
		Diagnostics: db2sdk.HCLDiagnostics(diagnostics),
	}
	if result != nil {
		response.Parameters = slice.List(result.Parameters, db2sdk.PreviewParameter)
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

func (api *API) handleParameterWebsocket(rw http.ResponseWriter, r *http.Request, initial codersdk.DynamicParametersRequest, render dynamicparameters.Renderer) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusUpgradeRequired, codersdk.Response{
			Message: "Failed to accept WebSocket.",
			Detail:  err.Error(),
		})
		return
	}
	ctx = api.wsWatcher.Watch(ctx, api.Logger, conn)

	stream := wsjson.NewStream[codersdk.DynamicParametersRequest, codersdk.DynamicParametersResponse](
		conn,
		websocket.MessageText,
		websocket.MessageText,
		api.Logger,
	)

	// Send an initial form state, computed without any user input.
	result, diagnostics := render.Render(ctx, initial.OwnerID, initial.Inputs)
	response := codersdk.DynamicParametersResponse{
		ID:          -1, // Always start with -1.
		Diagnostics: db2sdk.HCLDiagnostics(diagnostics),
	}
	if result != nil {
		response.Parameters = slice.List(result.Parameters, db2sdk.PreviewParameter)
	}
	err = stream.Send(response)
	if err != nil {
		stream.Drop()
		return
	}

	// As the user types into the form, reprocess the state using their input,
	// and respond with updates.
	updates := stream.Chan()
	ownerID := initial.OwnerID
	for {
		select {
		case <-ctx.Done():
			stream.Close(websocket.StatusGoingAway)
			return
		case update, ok := <-updates:
			if !ok {
				// The connection has been closed, so there is no one to write to
				return
			}

			// Take a nil uuid to mean the previous owner ID.
			// This just removes the need to constantly send who you are.
			if update.OwnerID == uuid.Nil {
				update.OwnerID = ownerID
			}

			ownerID = update.OwnerID

			result, diagnostics := render.Render(ctx, update.OwnerID, update.Inputs)
			response := codersdk.DynamicParametersResponse{
				ID:          update.ID,
				Diagnostics: db2sdk.HCLDiagnostics(diagnostics),
			}
			if result != nil {
				response.Parameters = slice.List(result.Parameters, db2sdk.PreviewParameter)
			}
			err = stream.Send(response)
			if err != nil {
				stream.Drop()
				return
			}
		}
	}
}
