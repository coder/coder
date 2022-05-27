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
	apiKey := httpmw.APIKey(r)
	// This requires the site wide action to create files.
	// Once created, a user can read their own files uploaded
	if !api.Authorize(rw, r, rbac.ActionCreate, rbac.ResourceFile) {
		return
	}

	contentType := r.Header.Get("Content-Type")

	switch contentType {
	case "application/x-tar":
	default:
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("unsupported content type: %s", contentType),
		})
		return
	}

	r.Body = http.MaxBytesReader(rw, r.Body, 10*(10<<20))
	data, err := io.ReadAll(r.Body)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("read file: %s", err),
		})
		return
	}
	hashBytes := sha256.Sum256(data)
	hash := hex.EncodeToString(hashBytes[:])
	file, err := api.Database.GetFileByHash(r.Context(), hash)
	if err == nil {
		// The file already exists!
		httpapi.Write(rw, http.StatusOK, codersdk.UploadResponse{
			Hash: file.Hash,
		})
		return
	}
	file, err = api.Database.InsertFile(r.Context(), database.InsertFileParams{
		Hash:      hash,
		CreatedBy: apiKey.UserID,
		CreatedAt: database.Now(),
		Mimetype:  contentType,
		Data:      data,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert file: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, codersdk.UploadResponse{
		Hash: file.Hash,
	})
}

func (api *API) fileByHash(rw http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "hash must be provided",
		})
		return
	}
	file, err := api.Database.GetFileByHash(r.Context(), hash)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get file: %s", err),
		})
		return
	}

	if !api.Authorize(rw, r, rbac.ActionRead,
		rbac.ResourceFile.WithOwner(file.CreatedBy.String()).WithID(file.Hash)) {
		return
	}

	rw.Header().Set("Content-Type", file.Mimetype)
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(file.Data)
}
