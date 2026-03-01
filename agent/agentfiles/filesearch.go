package agentfiles

import (
	"encoding/json"
	"net/http"

	"github.com/coder/coder/v2/agent/filefinder"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// HandleFileSearch handles file search requests using the filefinder
// engine. It returns a JSON object with a "results" array.
func (api *API) HandleFileSearch(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if api.filefinder == nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "File search is not available.",
			Detail:  "The file search engine has not been initialized.",
		})
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing query parameter.",
		})
		return
	}

	results, err := api.filefinder.Search(ctx, query, filefinder.SearchOptions{
		Limit: 20,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "File search failed.",
			Detail:  err.Error(),
		})
		return
	}

	resp := make([]fileSearchResult, 0, len(results))
	for _, r := range results {
		resp = append(resp, fileSearchResult{
			Path:  r.Path,
			IsDir: r.IsDir,
		})
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(fileSearchResponse{Results: resp})
}

type fileSearchResponse struct {
	Results []fileSearchResult `json:"results"`
}

type fileSearchResult struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}
