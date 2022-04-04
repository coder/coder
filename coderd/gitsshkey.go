package coderd

import (
	"net/http"

	"github.com/go-chi/render"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *api) regenerateGitSSHKey(rw http.ResponseWriter, r *http.Request) {
	var (
		user = httpmw.UserParam(r)
	)

	privateKey, publicKey, err := gitsshkey.GenerateKeyPair(api.SSHKeygenAlgorithm)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Could not regenerate key pair.",
		})
		return
	}

	err = api.Database.UpdateGitSSHKey(r.Context(), database.UpdateGitSSHKeyParams{
		UserID:     user.ID,
		UpdatedAt:  database.Now(),
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Could not update git ssh key.",
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "Updated git ssh key!",
	})
}

func (api *api) gitSSHKey(rw http.ResponseWriter, r *http.Request) {
	var (
		user = httpmw.UserParam(r)
	)

	gitSSHKey, err := api.Database.GetGitSSHKey(r.Context(), user.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Could not update git ssh key.",
		})
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, codersdk.GitSSHKey{
		UserID:    gitSSHKey.UserID,
		CreatedAt: gitSSHKey.CreatedAt,
		UpdatedAt: gitSSHKey.UpdatedAt,
		// No need to return the private key to the user
		PublicKey: gitSSHKey.PublicKey,
	})
}

func (api *api) privateGitSSHKey(rw http.ResponseWriter, r *http.Request) {
	// connect agent to workspace to user to gitsshkey
}
