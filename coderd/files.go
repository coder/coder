package coderd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/render"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

type UploadFileResponse struct {
	Hash string `json:"hash"`
}

func (api *api) postFiles(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
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
	file, err := api.Database.InsertFile(r.Context(), database.InsertFileParams{
		Hash:      hex.EncodeToString(hashBytes[:]),
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
	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, UploadFileResponse{
		Hash: file.Hash,
	})
}
