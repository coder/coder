package coderd

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func (api *API) postFile(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	// This requires the site wide action to create files.
	// Once created, a user can read their own files uploaded
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceFile.WithOwner(apiKey.UserID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	contentType := r.Header.Get("Content-Type")

	switch contentType {
	case "application/x-tar":
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Unsupported content type header %q.", contentType),
		})
		return
	}

	r.Body = http.MaxBytesReader(rw, r.Body, 10*(10<<20))
	data, err := io.ReadAll(r.Body)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read file from request.",
			Detail:  err.Error(),
		})
		return
	}
	hashBytes := sha256.Sum256(data)
	hash := hex.EncodeToString(hashBytes[:])
	file, err := api.Database.GetFileByHash(ctx, hash)
	if err == nil {
		// The file already exists!
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.UploadResponse{
			Hash: file.Hash,
		})
		return
	}
	file, err = api.Database.InsertFile(ctx, database.InsertFileParams{
		Hash:      hash,
		CreatedBy: apiKey.UserID,
		CreatedAt: database.Now(),
		Mimetype:  contentType,
		Data:      data,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error saving file.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.UploadResponse{
		Hash: file.Hash,
	})
}

func (api *API) fileByHash(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "File hash must be provided in url.",
		})
		return
	}
	file, err := api.Database.GetFileByHash(ctx, hash)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching file.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, rbac.ActionRead,
		rbac.ResourceFile.WithOwner(file.CreatedBy.String())) {
		// Return 404 to not leak the file exists
		httpapi.ResourceNotFound(rw)
		return
	}

	rw.Header().Set("Content-Type", file.Mimetype)
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(file.Data)
}
