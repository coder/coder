package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
)

// workspaceAppsAuthWildcard authenticates the wildcard domain.
func (api *API) workspaceAppsAuthWildcard(rw http.ResponseWriter, r *http.Request) {
	// r.URL.Query().Get("redirect")

}

func (api *API) workspaceAppsProxyWildcard(rw http.ResponseWriter, r *http.Request) {

}

func (api *API) workspaceAppsProxyPath(rw http.ResponseWriter, r *http.Request) {
	conn, err := api.dialWorkspaceAgent(r, uuid.Nil)
	if err != nil {
		return
	}
	app, err := api.Database.GetWorkspaceAppByAgentIDAndName(r.Context(), database.GetWorkspaceAppByAgentIDAndNameParams{
		AgentID: uuid.Nil,
		Name:    "something",
	})
	if err != nil {
		return
	}
	conn.DialContext(r.Context(), "tcp", "localhost:3000")
}
