package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func loginTypes() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(w, http.StatusOK, []codersdk.LoginType{
			{
				Type: "built-in",
			},
		})
	}
}
