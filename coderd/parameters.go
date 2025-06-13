package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/terraform-provider-coder/v2/provider"
	"github.com/coder/websocket"
)

// @Summary Evaluate dynamic parameters for template version
// @ID evaluate-dynamic-parameters-for-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Accept json
// @Produce json
// @Param request body codersdk.DynamicParametersRequest true "Initial parameter values"
// @Success 200 {object} codersdk.DynamicParametersResponse
// @Router /templateversions/{templateversion}/dynamic-parameters/evaluate [post]
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
// @Router /templateversions/{templateversion}/dynamic-parameters [get]
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

func (api *API) templateVersionDynamicParameters(listen bool, initial codersdk.DynamicParametersRequest) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		templateVersion := httpmw.TemplateVersionParam(r)

		// Check that the job has completed successfully
		job, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching provisioner job.",
				Detail:  err.Error(),
			})
			return
		}
		if !job.CompletedAt.Valid {
			httpapi.Write(ctx, rw, http.StatusTooEarly, codersdk.Response{
				Message: "Template version job has not finished",
			})
			return
		}

		tf, err := api.Database.GetTemplateVersionTerraformValues(ctx, templateVersion.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to retrieve Terraform values for template version",
				Detail:  err.Error(),
			})
			return
		}

		if wsbuilder.ProvisionerVersionSupportsDynamicParameters(tf.ProvisionerdVersion) {
			api.handleDynamicParameters(listen, rw, r, tf, templateVersion, initial)
		} else {
			api.handleStaticParameters(listen, rw, r, templateVersion.ID, initial)
		}
	}
}

type previewFunction func(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics)

// nolint:revive
func (api *API) handleDynamicParameters(listen bool, rw http.ResponseWriter, r *http.Request, tf database.TemplateVersionTerraformValue, templateVersion database.TemplateVersion, initial codersdk.DynamicParametersRequest) {
	var (
		ctx    = r.Context()
		apikey = httpmw.APIKey(r)
	)

	// nolint:gocritic // We need to fetch the templates files for the Terraform
	// evaluator, and the user likely does not have permission.
	fileCtx := dbauthz.AsProvisionerd(ctx)
	fileID, err := api.Database.GetFileIDByTemplateVersionID(fileCtx, templateVersion.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error finding template version Terraform.",
			Detail:  err.Error(),
		})
		return
	}

	// Add the file first. Calling `Release` if it fails is a no-op, so this is safe.
	templateFS, err := api.FileCache.Acquire(fileCtx, fileID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Internal error fetching template version Terraform.",
			Detail:  err.Error(),
		})
		return
	}
	defer api.FileCache.Release(fileID)

	// Having the Terraform plan available for the evaluation engine is helpful
	// for populating values from data blocks, but isn't strictly required. If
	// we don't have a cached plan available, we just use an empty one instead.
	plan := json.RawMessage("{}")
	if len(tf.CachedPlan) > 0 {
		plan = tf.CachedPlan
	}

	if tf.CachedModuleFiles.Valid {
		moduleFilesFS, err := api.FileCache.Acquire(fileCtx, tf.CachedModuleFiles.UUID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Internal error fetching Terraform modules.",
				Detail:  err.Error(),
			})
			return
		}
		defer api.FileCache.Release(tf.CachedModuleFiles.UUID)

		templateFS = files.NewOverlayFS(templateFS, []files.Overlay{{Path: ".terraform/modules", FS: moduleFilesFS}})
	}

	owner, err := getWorkspaceOwnerData(ctx, api.Database, apikey.UserID, templateVersion.OrganizationID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace owner.",
			Detail:  err.Error(),
		})
		return
	}

	input := preview.Input{
		PlanJSON:        plan,
		ParameterValues: map[string]string{},
		Owner:           owner,
	}

	// failedOwners keeps track of which owners failed to fetch from the database.
	// This prevents db spam on repeated requests for the same failed owner.
	failedOwners := make(map[uuid.UUID]error)
	failedOwnerDiag := hcl.Diagnostics{
		{
			Severity: hcl.DiagError,
			Summary:  "Failed to fetch workspace owner",
			Detail:   "Please check your permissions or the user may not exist.",
			Extra: previewtypes.DiagnosticExtra{
				Code: "owner_not_found",
			},
		},
	}

	dynamicRender := func(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
		if ownerID == uuid.Nil {
			// Default to the authenticated user
			// Nice for testing
			ownerID = apikey.UserID
		}

		if _, ok := failedOwners[ownerID]; ok {
			// If it has failed once, assume it will fail always.
			// Re-open the websocket to try again.
			return nil, failedOwnerDiag
		}

		// Update the input values with the new values.
		input.ParameterValues = values

		// Update the owner if there is a change
		if input.Owner.ID != ownerID.String() {
			owner, err = getWorkspaceOwnerData(ctx, api.Database, ownerID, templateVersion.OrganizationID)
			if err != nil {
				failedOwners[ownerID] = err
				return nil, failedOwnerDiag
			}
			input.Owner = owner
		}

		return preview.Preview(ctx, input, templateFS)
	}
	if listen {
		api.handleParameterWebsocket(rw, r, initial, dynamicRender)
	} else {
		api.handleParameterEvaluate(rw, r, initial, dynamicRender)
	}
}

// nolint:revive
func (api *API) handleStaticParameters(listen bool, rw http.ResponseWriter, r *http.Request, version uuid.UUID, initial codersdk.DynamicParametersRequest) {
	ctx := r.Context()
	dbTemplateVersionParameters, err := api.Database.GetTemplateVersionParameters(ctx, version)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to retrieve template version parameters",
			Detail:  err.Error(),
		})
		return
	}

	params := make([]previewtypes.Parameter, 0, len(dbTemplateVersionParameters))
	for _, it := range dbTemplateVersionParameters {
		param := previewtypes.Parameter{
			ParameterData: previewtypes.ParameterData{
				Name:         it.Name,
				DisplayName:  it.DisplayName,
				Description:  it.Description,
				Type:         previewtypes.ParameterType(it.Type),
				FormType:     "", // ooooof
				Styling:      previewtypes.ParameterStyling{},
				Mutable:      it.Mutable,
				DefaultValue: previewtypes.StringLiteral(it.DefaultValue),
				Icon:         it.Icon,
				Options:      make([]*previewtypes.ParameterOption, 0),
				Validations:  make([]*previewtypes.ParameterValidation, 0),
				Required:     it.Required,
				Order:        int64(it.DisplayOrder),
				Ephemeral:    it.Ephemeral,
				Source:       nil,
			},
			// Always use the default, since we used to assume the empty string
			Value:       previewtypes.StringLiteral(it.DefaultValue),
			Diagnostics: nil,
		}

		if it.ValidationError != "" || it.ValidationRegex != "" || it.ValidationMonotonic != "" {
			var reg *string
			if it.ValidationRegex != "" {
				reg = ptr.Ref(it.ValidationRegex)
			}

			var vMin *int64
			if it.ValidationMin.Valid {
				vMin = ptr.Ref(int64(it.ValidationMin.Int32))
			}

			var vMax *int64
			if it.ValidationMax.Valid {
				vMin = ptr.Ref(int64(it.ValidationMax.Int32))
			}

			var monotonic *string
			if it.ValidationMonotonic != "" {
				monotonic = ptr.Ref(it.ValidationMonotonic)
			}

			param.Validations = append(param.Validations, &previewtypes.ParameterValidation{
				Error:     it.ValidationError,
				Regex:     reg,
				Min:       vMin,
				Max:       vMax,
				Monotonic: monotonic,
			})
		}

		var protoOptions []*sdkproto.RichParameterOption
		_ = json.Unmarshal(it.Options, &protoOptions) // Not going to make this fatal
		for _, opt := range protoOptions {
			param.Options = append(param.Options, &previewtypes.ParameterOption{
				Name:        opt.Name,
				Description: opt.Description,
				Value:       previewtypes.StringLiteral(opt.Value),
				Icon:        opt.Icon,
			})
		}

		// Take the form type from the ValidateFormType function. This is a bit
		// unfortunate we have to do this, but it will return the default form_type
		// for a given set of conditions.
		_, param.FormType, _ = provider.ValidateFormType(provider.OptionType(param.Type), len(param.Options), param.FormType)

		param.Diagnostics = previewtypes.Diagnostics(param.Valid(param.Value))
		params = append(params, param)
	}

	staticRender := func(_ context.Context, _ uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
		for i := range params {
			param := &params[i]
			paramValue, ok := values[param.Name]
			if ok {
				param.Value = previewtypes.StringLiteral(paramValue)
			} else {
				param.Value = param.DefaultValue
			}
			param.Diagnostics = previewtypes.Diagnostics(param.Valid(param.Value))
		}

		return &preview.Output{
				Parameters: params,
			}, hcl.Diagnostics{
				{
					// Only a warning because the form does still work.
					Severity: hcl.DiagWarning,
					Summary:  "This template version is missing required metadata to support dynamic parameters.",
					Detail:   "To restore full functionality, please re-import the terraform as a new template version.",
				},
			}
	}
	if listen {
		api.handleParameterWebsocket(rw, r, initial, staticRender)
	} else {
		api.handleParameterEvaluate(rw, r, initial, staticRender)
	}
}

func (*API) handleParameterEvaluate(rw http.ResponseWriter, r *http.Request, initial codersdk.DynamicParametersRequest, render previewFunction) {
	ctx := r.Context()

	// Send an initial form state, computed without any user input.
	result, diagnostics := render(ctx, initial.OwnerID, initial.Inputs)
	response := codersdk.DynamicParametersResponse{
		ID:          0,
		Diagnostics: db2sdk.HCLDiagnostics(diagnostics),
	}
	if result != nil {
		response.Parameters = db2sdk.List(result.Parameters, db2sdk.PreviewParameter)
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

func (api *API) handleParameterWebsocket(rw http.ResponseWriter, r *http.Request, initial codersdk.DynamicParametersRequest, render previewFunction) {
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
	stream := wsjson.NewStream[codersdk.DynamicParametersRequest, codersdk.DynamicParametersResponse](
		conn,
		websocket.MessageText,
		websocket.MessageText,
		api.Logger,
	)

	// Send an initial form state, computed without any user input.
	result, diagnostics := render(ctx, initial.OwnerID, initial.Inputs)
	response := codersdk.DynamicParametersResponse{
		ID:          -1, // Always start with -1.
		Diagnostics: db2sdk.HCLDiagnostics(diagnostics),
	}
	if result != nil {
		response.Parameters = db2sdk.List(result.Parameters, db2sdk.PreviewParameter)
	}
	err = stream.Send(response)
	if err != nil {
		stream.Drop()
		return
	}

	// As the user types into the form, reprocess the state using their input,
	// and respond with updates.
	updates := stream.Chan()
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

			result, diagnostics := render(ctx, update.OwnerID, update.Inputs)
			response := codersdk.DynamicParametersResponse{
				ID:          update.ID,
				Diagnostics: db2sdk.HCLDiagnostics(diagnostics),
			}
			if result != nil {
				response.Parameters = db2sdk.List(result.Parameters, db2sdk.PreviewParameter)
			}
			err = stream.Send(response)
			if err != nil {
				stream.Drop()
				return
			}
		}
	}
}

func getWorkspaceOwnerData(
	ctx context.Context,
	db database.Store,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
) (previewtypes.WorkspaceOwner, error) {
	var g errgroup.Group

	// TODO: @emyrk we should only need read access on the org member, not the
	//   site wide user object. Figure out a better way to handle this.
	user, err := db.GetUserByID(ctx, ownerID)
	if err != nil {
		return previewtypes.WorkspaceOwner{}, xerrors.Errorf("fetch user: %w", err)
	}

	var ownerRoles []previewtypes.WorkspaceOwnerRBACRole
	g.Go(func() error {
		// nolint:gocritic // This is kind of the wrong query to use here, but it
		// matches how the provisioner currently works. We should figure out
		// something that needs less escalation but has the correct behavior.
		row, err := db.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), ownerID)
		if err != nil {
			return err
		}
		roles, err := row.RoleNames()
		if err != nil {
			return err
		}
		ownerRoles = make([]previewtypes.WorkspaceOwnerRBACRole, 0, len(roles))
		for _, it := range roles {
			if it.OrganizationID != uuid.Nil && it.OrganizationID != organizationID {
				continue
			}
			var orgID string
			if it.OrganizationID != uuid.Nil {
				orgID = it.OrganizationID.String()
			}
			ownerRoles = append(ownerRoles, previewtypes.WorkspaceOwnerRBACRole{
				Name:  it.Name,
				OrgID: orgID,
			})
		}
		return nil
	})

	var publicKey string
	g.Go(func() error {
		// The correct public key has to be sent. This will not be leaked
		// unless the template leaks it.
		// nolint:gocritic
		key, err := db.GetGitSSHKey(dbauthz.AsSystemRestricted(ctx), ownerID)
		if err != nil {
			return err
		}
		publicKey = key.PublicKey
		return nil
	})

	var groupNames []string
	g.Go(func() error {
		// The groups need to be sent to preview. These groups are not exposed to the
		// user, unless the template does it through the parameters. Regardless, we need
		// the correct groups, and a user might not have read access.
		// nolint:gocritic
		groups, err := db.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{
			OrganizationID: organizationID,
			HasMemberID:    ownerID,
		})
		if err != nil {
			return err
		}
		groupNames = make([]string, 0, len(groups))
		for _, it := range groups {
			groupNames = append(groupNames, it.Group.Name)
		}
		return nil
	})

	err = g.Wait()
	if err != nil {
		return previewtypes.WorkspaceOwner{}, err
	}

	return previewtypes.WorkspaceOwner{
		ID:           user.ID.String(),
		Name:         user.Username,
		FullName:     user.Name,
		Email:        user.Email,
		LoginType:    string(user.LoginType),
		RBACRoles:    ownerRoles,
		SSHPublicKey: publicKey,
		Groups:       groupNames,
	}, nil
}
