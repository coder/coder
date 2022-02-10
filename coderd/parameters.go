package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

// ParameterSchema represents a parameter parsed from project version source.
type ParameterSchema database.ParameterSchema

// ParameterValue represents a set value for the scope.
type ParameterValue database.ParameterValue

// ComputedParameterValue represents a computed parameter value.
type ComputedParameterValue parameter.ComputedValue

// CreateParameterValueRequest is used to create a new parameter value for a scope.
type CreateParameterValueRequest struct {
	Name              string                              `json:"name" validate:"required"`
	SourceValue       string                              `json:"source_value" validate:"required"`
	SourceScheme      database.ParameterSourceScheme      `json:"source_scheme" validate:"oneof=data,required"`
	DestinationScheme database.ParameterDestinationScheme `json:"destination_scheme" validate:"oneof=environment_variable provisioner_variable,required"`
}

// Abstracts creating parameters into a single request/response format.
// Callers are in charge of validating the requester has permissions to
// perform the creation.
func postParameterValueForScope(rw http.ResponseWriter, r *http.Request, db database.Store, scope database.ParameterScope, scopeID string) {
	var createRequest CreateParameterValueRequest
	if !httpapi.Read(rw, r, &createRequest) {
		return
	}
	parameterValue, err := db.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
		ID:                uuid.New(),
		Name:              createRequest.Name,
		CreatedAt:         database.Now(),
		UpdatedAt:         database.Now(),
		Scope:             scope,
		ScopeID:           scopeID,
		SourceScheme:      createRequest.SourceScheme,
		SourceValue:       createRequest.SourceValue,
		DestinationScheme: createRequest.DestinationScheme,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert parameter value: %s", err),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, parameterValue)
}

// Abstracts returning parameters for a scope into a standardized
// request/response format. Callers are responsible for checking
// requester permissions.
func parametersForScope(rw http.ResponseWriter, r *http.Request, db database.Store, req database.GetParameterValuesByScopeParams) {
	parameterValues, err := db.GetParameterValuesByScope(r.Context(), req)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		parameterValues = []database.ParameterValue{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get parameter values: %s", err),
		})
		return
	}

	apiParameterValues := make([]ParameterValue, 0, len(parameterValues))
	for _, parameterValue := range parameterValues {
		apiParameterValues = append(apiParameterValues, convertParameterValue(parameterValue))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiParameterValues)
}

// Returns parameters for a specific scope.
func computedParametersForScope(rw http.ResponseWriter, r *http.Request, db database.Store, scope parameter.ComputeScope) {
	values, err := parameter.Compute(r.Context(), db, scope, &parameter.ComputeOptions{
		// We *never* want to send the client secret parameter values.
		HideRedisplayValues: true,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("compute values: %s", err),
		})
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, values)
}

func convertParameterValue(parameterValue database.ParameterValue) ParameterValue {
	parameterValue.SourceValue = ""
	return ParameterValue(parameterValue)
}
