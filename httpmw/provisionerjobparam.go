package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

type provisionerJobParamContextKey struct{}

// ProvisionerJobParam returns the project from the ExtractProjectParam handler.
func ProvisionerJobParam(r *http.Request) database.ProvisionerJob {
	provisionerJob, ok := r.Context().Value(provisionerJobParamContextKey{}).(database.ProvisionerJob)
	if !ok {
		panic("developer error: provisioner job param middleware not provided")
	}
	return provisionerJob
}

// ExtractProvisionerJobParam grabs a provisioner job from the "provisionerjob" URL parameter.
func ExtractProvisionerJobParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			jobID := chi.URLParam(r, "provisionerjob")
			if jobID == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "provisioner job must be provided",
				})
				return
			}
			jobUUID, err := uuid.Parse(jobID)
			if err != nil {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "job id must be a uuid",
				})
				return
			}
			job, err := db.GetProvisionerJobByID(r.Context(), jobUUID)
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
					Message: "job doesn't exist with that id",
				})
				return
			}
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get provisioner job: %s", err),
				})
				return
			}

			ctx := context.WithValue(r.Context(), provisionerJobParamContextKey{}, job)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
