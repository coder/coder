package coderd

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/usersecretspubsub"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

const initialDynamicParametersResponseID = -1

// @Summary Evaluate dynamic parameters for template version
// @ID evaluate-dynamic-parameters-for-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Accept json
// @Produce json
// @Param request body codersdk.DynamicParametersRequest true "Initial parameter values"
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
// @ID open-dynamic-parameters-websocket-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
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
		ID:      initialDynamicParametersResponseID,
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
			dynamicparameters.WithLogger(api.Logger.Named("dynamicparameters")),
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
	response := renderDynamicParametersResponse(ctx, render, 0, initial.OwnerID, initial.Inputs)
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
	go httpapi.HeartbeatClose(ctx, api.Logger, cancel, conn)

	stream := wsjson.NewStream[codersdk.DynamicParametersRequest, codersdk.DynamicParametersResponse](
		conn,
		websocket.MessageText,
		websocket.MessageText,
		api.Logger,
	)

	secretEvents := make(chan uuid.UUID, 1)
	secretSubscriber := &parameterSecretEventSubscriber{
		api:    api,
		events: secretEvents,
	}
	secretSubscriber.UpdateOwnerSubscription(ctx, initial.OwnerID)
	defer secretSubscriber.Close()

	sender := dynamicParametersResponseSender{
		stream: stream,
		render: render,
	}

	// Send an initial form state, computed without any user input.
	if !sender.Send(ctx, initialDynamicParametersResponseID, initial.OwnerID, initial.Inputs) {
		return
	}

	// As the user types into the form or updates secrets in another client,
	// reprocess the state using their input and respond with updates.
	updates := stream.Chan()
	ownerID := initial.OwnerID
	inputs := initial.Inputs
	lastResponseID := initialDynamicParametersResponseID
	for {
		select {
		case <-ctx.Done():
			stream.Close(websocket.StatusGoingAway)
			return
		case eventOwnerID := <-secretEvents:
			if eventOwnerID != ownerID {
				continue
			}
			lastResponseID = nextDynamicParametersResponseID(lastResponseID, lastResponseID+1)
			if !sender.Send(ctx, lastResponseID, ownerID, inputs) {
				return
			}
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
			inputs = update.Inputs
			secretSubscriber.UpdateOwnerSubscription(ctx, ownerID)
			responseID := nextDynamicParametersResponseID(lastResponseID, update.ID)
			lastResponseID = responseID
			if !sender.Send(ctx, responseID, ownerID, inputs) {
				return
			}
		}
	}
}

func renderDynamicParametersResponse(
	ctx context.Context,
	render dynamicparameters.Renderer,
	id int,
	ownerID uuid.UUID,
	inputs map[string]string,
) codersdk.DynamicParametersResponse {
	result, diagnostics := render.Render(ctx, ownerID, inputs, dynamicparameters.IncludeSecretRequirements())
	response := codersdk.DynamicParametersResponse{
		ID:          id,
		Diagnostics: db2sdk.HCLDiagnostics(diagnostics),
	}
	if result.Output != nil {
		response.Parameters = slice.List(result.Output.Parameters, db2sdk.PreviewParameter)
	}
	response.SecretRequirements = result.SecretRequirements
	return response
}

type dynamicParametersResponseSender struct {
	stream *wsjson.Stream[codersdk.DynamicParametersRequest, codersdk.DynamicParametersResponse]
	render dynamicparameters.Renderer
}

func (s dynamicParametersResponseSender) Send(
	ctx context.Context,
	id int,
	ownerID uuid.UUID,
	inputs map[string]string,
) bool {
	response := renderDynamicParametersResponse(ctx, s.render, id, ownerID, inputs)
	if err := s.stream.Send(response); err != nil {
		s.stream.Drop()
		return false
	}
	return true
}

type parameterSecretEventSubscriber struct {
	api    *API
	events chan uuid.UUID

	cancel  func()
	ownerID uuid.UUID
}

// UpdateOwnerSubscription switches the pubsub subscription to the owner's
// user secret channel. Dynamic parameters can render for a workspace owner
// other than the connected user, so owner changes must update the channel
// that drives secret requirement refreshes.
func (s *parameterSecretEventSubscriber) UpdateOwnerSubscription(ctx context.Context, ownerID uuid.UUID) {
	if ownerID == s.ownerID {
		return
	}
	if s.cancel != nil {
		s.Close()
	}
	// Websocket authorization uses the actor snapshot from connection
	// creation, matching the rest of the websocket handlers.
	if !s.api.canSubscribeUserSecretEvents(ctx, ownerID) {
		s.ownerID = ownerID
		return
	}
	s.ownerID = ownerID
	subscribedOwnerID := ownerID
	cancel, err := s.api.Pubsub.Subscribe(usersecretspubsub.Channel(ownerID), func(context.Context, []byte) {
		s.notify(subscribedOwnerID)
	})
	if err != nil {
		// Leave the owner unset so transient pubsub failures can be
		// retried on the next update for this owner.
		s.ownerID = uuid.Nil
		s.api.Logger.Warn(ctx, "failed to subscribe to user secret events",
			slog.F("user_id", ownerID),
			slog.Error(err),
		)
		return
	}
	s.cancel = cancel
}

func (s *parameterSecretEventSubscriber) Close() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	s.cancel = nil
}

func (s *parameterSecretEventSubscriber) notify(ownerID uuid.UUID) {
	select {
	case s.events <- ownerID:
	default:
	}
}

func nextDynamicParametersResponseID(lastResponseID int, requestID int) int {
	if requestID <= lastResponseID {
		return lastResponseID + 1
	}
	return requestID
}

func (api *API) canSubscribeUserSecretEvents(ctx context.Context, ownerID uuid.UUID) bool {
	roles, ok := dbauthz.ActorFromContext(ctx)
	if !ok {
		api.Logger.Error(ctx, "no authorization actor for user secret event subscription")
		return false
	}
	return api.HTTPAuth.Authorizer.Authorize(
		ctx,
		roles,
		policy.ActionRead,
		rbac.ResourceUserSecret.WithOwner(ownerID.String()),
	) == nil
}
