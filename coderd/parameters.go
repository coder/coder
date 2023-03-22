package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/codersdk"
)

// @Summary Create parameter
// @ID create-parameter
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Parameters
// @Param request body codersdk.CreateParameterRequest true "Parameter request"
// @Param scope path string true "Scope" Enums(template,workspace,import_job)
// @Param id path string true "ID" format(uuid)
// @Success 201 {object} codersdk.Parameter
// @Router /parameters/{scope}/{id} [post]
func (api *API) postParameter(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scope, scopeID, valid := readScopeAndID(ctx, rw, r)
	if !valid {
		return
	}

	var createRequest codersdk.CreateParameterRequest
	if !httpapi.Read(ctx, rw, r, &createRequest) {
		return
	}
	_, err := api.Database.GetParameterValueByScopeAndName(ctx, database.GetParameterValueByScopeAndNameParams{
		Scope:   scope,
		ScopeID: scopeID,
		Name:    createRequest.Name,
	})
	if err == nil {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Parameter already exists in scope %q and name %q.", scope, createRequest.Name),
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching parameter.",
			Detail:  err.Error(),
		})
		return
	}

	parameterValue, err := api.Database.InsertParameterValue(ctx, database.InsertParameterValueParams{
		ID:                uuid.New(),
		Name:              createRequest.Name,
		CreatedAt:         database.Now(),
		UpdatedAt:         database.Now(),
		Scope:             scope,
		ScopeID:           scopeID,
		SourceScheme:      database.ParameterSourceScheme(createRequest.SourceScheme),
		SourceValue:       createRequest.SourceValue,
		DestinationScheme: database.ParameterDestinationScheme(createRequest.DestinationScheme),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error inserting parameter.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertParameterValue(parameterValue))
}

// @Summary Get parameters
// @ID get-parameters
// @Security CoderSessionToken
// @Produce json
// @Tags Parameters
// @Param scope path string true "Scope" Enums(template,workspace,import_job)
// @Param id path string true "ID" format(uuid)
// @Success 200 {array} codersdk.Parameter
// @Router /parameters/{scope}/{id} [get]
func (api *API) parameters(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scope, scopeID, valid := readScopeAndID(ctx, rw, r)
	if !valid {
		return
	}

	parameterValues, err := api.Database.ParameterValues(ctx, database.ParameterValuesParams{
		Scopes:   []database.ParameterScope{scope},
		ScopeIds: []uuid.UUID{scopeID},
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching parameter scope values.",
			Detail:  err.Error(),
		})
		return
	}
	apiParameterValues := make([]codersdk.Parameter, 0, len(parameterValues))
	for _, parameterValue := range parameterValues {
		apiParameterValues = append(apiParameterValues, convertParameterValue(parameterValue))
	}

	httpapi.Write(ctx, rw, http.StatusOK, apiParameterValues)
}

// @Summary Delete parameter
// @ID delete-parameter
// @Security CoderSessionToken
// @Produce json
// @Tags Parameters
// @Param scope path string true "Scope" Enums(template,workspace,import_job)
// @Param id path string true "ID" format(uuid)
// @Param name path string true "Name"
// @Success 200 {object} codersdk.Response
// @Router /parameters/{scope}/{id}/{name} [delete]
func (api *API) deleteParameter(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scope, scopeID, valid := readScopeAndID(ctx, rw, r)
	if !valid {
		return
	}

	name := chi.URLParam(r, "name")
	parameterValue, err := api.Database.GetParameterValueByScopeAndName(ctx, database.GetParameterValueByScopeAndNameParams{
		Scope:   scope,
		ScopeID: scopeID,
		Name:    name,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching parameter.",
			Detail:  err.Error(),
		})
		return
	}
	err = api.Database.DeleteParameterValueByID(ctx, parameterValue.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting parameter.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Parameter deleted.",
	})
}

func convertParameterSchema(parameterSchema database.ParameterSchema) (codersdk.ParameterSchema, error) {
	contains := []string{}
	if parameterSchema.ValidationCondition != "" {
		var err error
		contains, _, err = parameter.Contains(parameterSchema.ValidationCondition)
		if err != nil {
			return codersdk.ParameterSchema{}, xerrors.Errorf("parse validation condition for %q: %w", parameterSchema.Name, err)
		}
	}

	return codersdk.ParameterSchema{
		ID:                       parameterSchema.ID,
		CreatedAt:                parameterSchema.CreatedAt,
		JobID:                    parameterSchema.JobID,
		Name:                     parameterSchema.Name,
		Description:              parameterSchema.Description,
		DefaultSourceScheme:      codersdk.ParameterSourceScheme(parameterSchema.DefaultSourceScheme),
		DefaultSourceValue:       parameterSchema.DefaultSourceValue,
		AllowOverrideSource:      parameterSchema.AllowOverrideSource,
		DefaultDestinationScheme: codersdk.ParameterDestinationScheme(parameterSchema.DefaultDestinationScheme),
		AllowOverrideDestination: parameterSchema.AllowOverrideDestination,
		DefaultRefresh:           parameterSchema.DefaultRefresh,
		RedisplayValue:           parameterSchema.RedisplayValue,
		ValidationError:          parameterSchema.ValidationError,
		ValidationCondition:      parameterSchema.ValidationCondition,
		ValidationTypeSystem:     string(parameterSchema.ValidationTypeSystem),
		ValidationValueType:      parameterSchema.ValidationValueType,
		ValidationContains:       contains,
	}, nil
}

func convertParameterValue(parameterValue database.ParameterValue) codersdk.Parameter {
	return codersdk.Parameter{
		ID:                parameterValue.ID,
		CreatedAt:         parameterValue.CreatedAt,
		UpdatedAt:         parameterValue.UpdatedAt,
		Scope:             codersdk.ParameterScope(parameterValue.Scope),
		ScopeID:           parameterValue.ScopeID,
		Name:              parameterValue.Name,
		SourceScheme:      codersdk.ParameterSourceScheme(parameterValue.SourceScheme),
		DestinationScheme: codersdk.ParameterDestinationScheme(parameterValue.DestinationScheme),
	}
}

func readScopeAndID(ctx context.Context, rw http.ResponseWriter, r *http.Request) (database.ParameterScope, uuid.UUID, bool) {
	scope := database.ParameterScope(chi.URLParam(r, "scope"))
	switch scope {
	case database.ParameterScopeTemplate, database.ParameterScopeImportJob, database.ParameterScopeWorkspace:
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid scope %q.", scope),
			Validations: []codersdk.ValidationError{
				{Field: "scope", Detail: "invalid scope"},
			},
		})
		return scope, uuid.Nil, false
	}

	id := chi.URLParam(r, "id")
	uid, err := uuid.Parse(id)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid UUID %q.", id),
			Detail:  err.Error(),
			Validations: []codersdk.ValidationError{
				{Field: "id", Detail: "Invalid UUID"},
			},
		})
		return scope, uuid.Nil, false
	}

	return scope, uid, true
}
