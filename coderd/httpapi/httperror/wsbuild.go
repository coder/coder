package httperror

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// WriteWorkspaceBuildError renders err as a workspace-build-shaped 4xx
// envelope when possible. Errors that implement WorkspaceBuildResponder
// emit the kinded envelope. Errors that only implement Responder are
// promoted into the kinded envelope shape with an empty Kind on each
// validation entry. Unrecognized errors fall through to a 500.
func WriteWorkspaceBuildError(ctx context.Context, rw http.ResponseWriter, err error) {
	if responseErr, ok := isWorkspaceBuildResponder(err); ok {
		code, resp := responseErr.BuildResponse()
		httpapi.Write(ctx, rw, code, resp)
		return
	}

	if responseErr, ok := IsResponder(err); ok {
		code, resp := responseErr.Response()
		httpapi.Write(ctx, rw, code, promoteResponse(resp))
		return
	}

	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.WorkspaceBuildErrorResponse{
		Message: "Internal error creating workspace build.",
		Detail:  err.Error(),
	})
}

// promoteResponse converts a codersdk.Response into the workspace-build
// envelope, preserving validations with an empty Kind on each entry.
// Used for errors that implement Responder but not
// WorkspaceBuildResponder.
func promoteResponse(resp codersdk.Response) codersdk.WorkspaceBuildErrorResponse {
	promoted := codersdk.WorkspaceBuildErrorResponse{
		Message: resp.Message,
		Detail:  resp.Detail,
	}
	if len(resp.Validations) > 0 {
		promoted.Validations = make([]codersdk.WorkspaceBuildValidationError, 0, len(resp.Validations))
		for _, v := range resp.Validations {
			promoted.Validations = append(promoted.Validations, codersdk.WorkspaceBuildValidationError{
				Field:  v.Field,
				Detail: v.Detail,
			})
		}
	}
	return promoted
}
