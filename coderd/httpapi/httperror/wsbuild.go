package httperror

import (
	"context"
	"errors"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
)

func WriteWorkspaceBuildError(ctx context.Context, rw http.ResponseWriter, err error) {
	if responseErr, ok := IsCoderSDKError(err); ok {
		code, resp := responseErr.Response()

		httpapi.Write(ctx, rw, code, resp)
		return
	}

	var buildErr wsbuilder.BuildError
	if errors.As(err, &buildErr) {
		if httpapi.IsUnauthorizedError(err) {
			buildErr.Status = http.StatusForbidden
		}

		httpapi.Write(ctx, rw, buildErr.Status, codersdk.Response{
			Message: buildErr.Message,
			Detail:  buildErr.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
		Message: "Internal error creating workspace build.",
		Detail:  err.Error(),
	})
}
