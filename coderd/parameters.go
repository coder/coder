package coderd

import (
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

func (api *API) postParameter(rw http.ResponseWriter, r *http.Request) {
	var createRequest codersdk.CreateParameterRequest
	if !httpapi.Read(rw, r, &createRequest) {
		return
	}
	scope, scopeID, valid := readScopeAndID(rw, r)
	if !valid {
		return
	}
	_, err := api.Database.GetParameterValueByScopeAndName(r.Context(), database.GetParameterValueByScopeAndNameParams{
		Scope:   scope,
		ScopeID: scopeID,
		Name:    createRequest.Name,
	})
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("a parameter already exists in scope %q with name %q", scope, createRequest.Name),
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get parameter value: %s", err),
		})
		return
	}
	parameterValue, err := api.Database.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
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
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert parameter value: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertParameterValue(parameterValue))
}

func (api *API) parameters(rw http.ResponseWriter, r *http.Request) {
	scope, scopeID, valid := readScopeAndID(rw, r)
	if !valid {
		return
	}
	parameterValues, err := api.Database.GetParameterValuesByScope(r.Context(), database.GetParameterValuesByScopeParams{
		Scope:   scope,
		ScopeID: scopeID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get parameter values by scope: %s", err),
		})
		return
	}
	apiParameterValues := make([]codersdk.Parameter, 0, len(parameterValues))
	for _, parameterValue := range parameterValues {
		apiParameterValues = append(apiParameterValues, convertParameterValue(parameterValue))
	}

	httpapi.Write(rw, http.StatusOK, apiParameterValues)
}

func (api *API) deleteParameter(rw http.ResponseWriter, r *http.Request) {
	scope, scopeID, valid := readScopeAndID(rw, r)
	if !valid {
		return
	}
	name := chi.URLParam(r, "name")
	parameterValue, err := api.Database.GetParameterValueByScopeAndName(r.Context(), database.GetParameterValueByScopeAndNameParams{
		Scope:   scope,
		ScopeID: scopeID,
		Name:    name,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("parameter doesn't exist in the provided scope with name %q", name),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get parameter value: %s", err),
		})
		return
	}
	err = api.Database.DeleteParameterValueByID(r.Context(), parameterValue.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("delete parameter: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "parameter deleted",
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

func readScopeAndID(rw http.ResponseWriter, r *http.Request) (database.ParameterScope, uuid.UUID, bool) {
	var scope database.ParameterScope
	switch chi.URLParam(r, "scope") {
	case string(codersdk.ParameterOrganization):
		scope = database.ParameterScopeOrganization
	case string(codersdk.ParameterTemplate):
		scope = database.ParameterScopeTemplate
	case string(codersdk.ParameterUser):
		scope = database.ParameterScopeUser
	case string(codersdk.ParameterWorkspace):
		scope = database.ParameterScopeWorkspace
	default:
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("invalid scope %q", scope),
		})
		return scope, uuid.Nil, false
	}

	id := chi.URLParam(r, "id")
	uid, err := uuid.Parse(id)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("invalid uuid %q: %s", id, err),
		})
		return scope, uuid.Nil, false
	}

	return scope, uid, true
}
