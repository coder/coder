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
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

const (
	tarMimeType = "application/x-tar"
)

// @Summary Upload file
// @Description Swagger notice: Swagger 2.0 doesn't support file upload with a `content-type` different than `application/x-www-form-urlencoded`.
// @ID upload-file
// @Security CoderSessionToken
// @Produce json
// @Accept application/x-tar
// @Tags Files
// @Param Content-Type header string true "Content-Type must be `application/x-tar`" default(application/x-tar)
// @Param file formData file true "File to be uploaded"
// @Success 201 {object} codersdk.UploadResponse
// @Router /files [post]
func (api *API) postFile(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	contentType := r.Header.Get("Content-Type")

	switch contentType {
	case tarMimeType:
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
	file, err := api.Database.GetFileByHashAndCreator(ctx, database.GetFileByHashAndCreatorParams{
		Hash:      hash,
		CreatedBy: apiKey.UserID,
	})
	if err == nil {
		// The file already exists!
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.UploadResponse{
			ID: file.ID,
		})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting file.",
			Detail:  err.Error(),
		})
		return
	}

	id := uuid.New()
	file, err = api.Database.InsertFile(ctx, database.InsertFileParams{
		ID:        id,
		Hash:      hash,
		CreatedBy: apiKey.UserID,
		CreatedAt: dbtime.Now(),
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
		ID: file.ID,
	})
}

// @Summary Get file by ID
// @ID get-file-by-id
// @Security CoderSessionToken
// @Tags Files
// @Param fileID path string true "File ID" format(uuid)
// @Success 200
// @Router /files/{fileID} [get]
func (api *API) fileByID(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fileID := chi.URLParam(r, "fileID")
	if fileID == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "File id must be provided in url.",
		})
		return
	}

	id, err := uuid.Parse(fileID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "File id must be a valid UUID.",
		})
		return
	}

	file, err := api.Database.GetFileByID(ctx, id)
	if httpapi.Is404Error(err) {
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

	rw.Header().Set("Content-Type", file.Mimetype)
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(file.Data)
}
