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

	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/websocket"
)

// @Summary Open dynamic parameters WebSocket by template version
// @ID open-dynamic-parameters-websocket-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param user path string true "Template version ID" format(uuid)
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 101
// @Router /users/{user}/templateversions/{templateversion}/parameters [get]
func (api *API) templateVersionDynamicParameters(rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	user := httpmw.UserParam(r)
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

	staticDiagnostics := parameterProvisionerVersionDiagnostic(tf)

	var render previewFunction
	major, minor, err := apiversion.Parse(tf.ProvisionerdVersion)
	if err != nil || major < 1 || (major == 1 && minor < 5) {
		staticRender, err := prepareStaticPreview(ctx, api.Database, templateVersion.ID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to setup static rendering",
				Detail:  err.Error(),
			})
			return
		}
		render = staticRender
	} else {
		// If the major version is 1.5+, we can use the dynamic preview
		dynamicRender, closer, success := prepareDynamicPreview(ctx, rw, api.Database, api.FileCache, tf, templateVersion, user)
		if !success {
			return
		}
		defer closer()
		render = dynamicRender
	}

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
	result, diagnostics := render(ctx, map[string]string{})
	response := codersdk.DynamicParametersResponse{
		ID:          -1,
		Diagnostics: previewtypes.Diagnostics(diagnostics.Extend(staticDiagnostics)),
	}
	if result != nil {
		response.Parameters = result.Parameters
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
			result, diagnostics := render(ctx, update.Inputs)
			response := codersdk.DynamicParametersResponse{
				ID:          update.ID,
				Diagnostics: previewtypes.Diagnostics(diagnostics.Extend(staticDiagnostics)),
			}
			if result != nil {
				response.Parameters = result.Parameters
			}
			err = stream.Send(response)
			if err != nil {
				stream.Drop()
				return
			}
		}
	}
}

type previewFunction func(ctx context.Context, values map[string]string) (*preview.Output, hcl.Diagnostics)

func prepareDynamicPreview(ctx context.Context, rw http.ResponseWriter, db database.Store, fc *files.Cache, tf database.TemplateVersionTerraformValue, templateVersion database.TemplateVersion, user database.User) (render previewFunction, closer func(), success bool) {
	openFiles := make([]uuid.UUID, 0)
	closeFiles := func() {
		for _, it := range openFiles {
			fc.Release(it)
		}
	}
	defer func() {
		if !success {
			closeFiles()
		}
	}()

	// nolint:gocritic // We need to fetch the templates files for the Terraform
	// evaluator, and the user likely does not have permission.
	fileCtx := dbauthz.AsProvisionerd(ctx)
	fileID, err := db.GetFileIDByTemplateVersionID(fileCtx, templateVersion.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error finding template version Terraform.",
			Detail:  err.Error(),
		})
		return nil, nil, false
	}

	templateFS, err := fc.Acquire(fileCtx, fileID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Internal error fetching template version Terraform.",
			Detail:  err.Error(),
		})
		return nil, nil, false
	}
	openFiles = append(openFiles, fileID)

	// Having the Terraform plan available for the evaluation engine is helpful
	// for populating values from data blocks, but isn't strictly required. If
	// we don't have a cached plan available, we just use an empty one instead.
	plan := json.RawMessage("{}")
	plan = tf.CachedPlan

	if tf.CachedModuleFiles.Valid {
		moduleFilesFS, err := fc.Acquire(fileCtx, tf.CachedModuleFiles.UUID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Internal error fetching Terraform modules.",
				Detail:  err.Error(),
			})
			return nil, nil, false
		}
		openFiles = append(openFiles, tf.CachedModuleFiles.UUID)

		templateFS = files.NewOverlayFS(templateFS, []files.Overlay{{Path: ".terraform/modules", FS: moduleFilesFS}})
	}

	owner, err := getWorkspaceOwnerData(ctx, db, user, templateVersion.OrganizationID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace owner.",
			Detail:  err.Error(),
		})
		return nil, nil, false
	}

	input := preview.Input{
		PlanJSON:        plan,
		ParameterValues: map[string]string{},
		Owner:           owner,
	}

	return func(ctx context.Context, values map[string]string) (*preview.Output, hcl.Diagnostics) {
		return preview.Preview(ctx, input, templateFS)
	}, closeFiles, true
}

func prepareStaticPreview(ctx context.Context, db database.Store, version uuid.UUID) (previewFunction, error) {
	dbTemplateVersionParameters, err := db.GetTemplateVersionParameters(ctx, version)
	if err != nil {
		return nil, xerrors.Errorf("error fetching template version parameters: %w", err)
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
				Options:      nil,
				Validations:  nil,
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

		param.Diagnostics = previewtypes.Diagnostics(param.Valid(param.Value))
		params = append(params, param)
	}

	return func(ctx context.Context, values map[string]string) (*preview.Output, hcl.Diagnostics) {
		for i := range params {
			param := &params[i]
			paramValue, ok := values[param.Name]
			if ok {
				param.Value = previewtypes.StringLiteral(paramValue)
			} else {
				paramValue = param.DefaultValue.AsString()
			}
			param.Diagnostics = previewtypes.Diagnostics(param.Valid(param.Value))
		}

		return &preview.Output{
			Parameters: params,
		}, nil
	}, nil
}

func getWorkspaceOwnerData(
	ctx context.Context,
	db database.Store,
	user database.User,
	organizationID uuid.UUID,
) (previewtypes.WorkspaceOwner, error) {
	var g errgroup.Group

	var ownerRoles []previewtypes.WorkspaceOwnerRBACRole
	g.Go(func() error {
		// nolint:gocritic // This is kind of the wrong query to use here, but it
		// matches how the provisioner currently works. We should figure out
		// something that needs less escalation but has the correct behavior.
		row, err := db.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), user.ID)
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
		key, err := db.GetGitSSHKey(ctx, user.ID)
		if err != nil {
			return err
		}
		publicKey = key.PublicKey
		return nil
	})

	var groupNames []string
	g.Go(func() error {
		groups, err := db.GetGroups(ctx, database.GetGroupsParams{
			OrganizationID: organizationID,
			HasMemberID:    user.ID,
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

	err := g.Wait()
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

// parameterProvisionerVersionDiagnostic checks the version of the provisioner
// used to create the template version. If the version is less than 1.5, it
// returns a warning diagnostic. Only versions 1.5+ return the module & plan data
// required.
func parameterProvisionerVersionDiagnostic(tf database.TemplateVersionTerraformValue) hcl.Diagnostics {
	missingMetadata := hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "This template version is missing required metadata to support dynamic parameters. Go back to the classic creation flow.",
		Detail:   "To restore full functionality, please re-import the terraform as a new template version.",
	}

	if tf.ProvisionerdVersion == "" {
		return hcl.Diagnostics{&missingMetadata}
	}

	major, minor, err := apiversion.Parse(tf.ProvisionerdVersion)
	if err != nil || tf.ProvisionerdVersion == "" {
		return hcl.Diagnostics{&missingMetadata}
	} else if major < 1 || (major == 1 && minor < 5) {
		missingMetadata.Detail = "This template version does not support dynamic parameters. " +
			"Some options may be missing or incorrect. " +
			"Please contact an administrator to update the provisioner and re-import the template version."
		return hcl.Diagnostics{&missingMetadata}
	}

	return nil
}
