package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
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

	fs, err := api.FileCache.Acquire(fileCtx, fileID)
	defer api.FileCache.Release(fileID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Internal error fetching template version Terraform.",
			Detail:  err.Error(),
		})
		return
	}

	// Having the Terraform plan available for the evaluation engine is helpful
	// for populating values from data blocks, but isn't strictly required. If
	// we don't have a cached plan available, we just use an empty one instead.
	plan := json.RawMessage("{}")
	tf, err := api.Database.GetTemplateVersionTerraformValues(ctx, templateVersion.ID)
	if err == nil {
		plan = tf.CachedPlan
	} else if !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to retrieve Terraform values for template version",
			Detail:  err.Error(),
		})
		return
	}

	owner, err := api.getWorkspaceOwnerData(ctx, user, templateVersion.OrganizationID)
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
	result, diagnostics := preview.Preview(ctx, input, fs)
	response := codersdk.DynamicParametersResponse{
		ID:          -1,
		Diagnostics: previewtypes.Diagnostics(diagnostics),
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
			input.ParameterValues = update.Inputs
			result, diagnostics := preview.Preview(ctx, input, fs)
			response := codersdk.DynamicParametersResponse{
				ID:          update.ID,
				Diagnostics: previewtypes.Diagnostics(diagnostics),
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

func (api *API) getWorkspaceOwnerData(
	ctx context.Context,
	user database.User,
	organizationID uuid.UUID,
) (previewtypes.WorkspaceOwner, error) {
	var g errgroup.Group

	var ownerRoles []previewtypes.WorkspaceOwnerRBACRole
	g.Go(func() error {
		// nolint:gocritic // This is kind of the wrong query to use here, but it
		// matches how the provisioner currently works. We should figure out
		// something that needs less escalation but has the correct behavior.
		row, err := api.Database.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), user.ID)
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
		key, err := api.Database.GetGitSSHKey(ctx, user.ID)
		if err != nil {
			return err
		}
		publicKey = key.PublicKey
		return nil
	})

	var groupNames []string
	g.Go(func() error {
		groups, err := api.Database.GetGroups(ctx, database.GetGroupsParams{
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
