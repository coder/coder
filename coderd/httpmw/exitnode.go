package httpmw

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type exitNodeContextKey struct{}

// ExitNodeFromContext returns the exit node stored by ExtractExitNode.
func ExitNodeFromContext(ctx context.Context) (database.ExitNode, bool) {
	node, ok := ctx.Value(exitNodeContextKey{}).(database.ExitNode)
	return node, ok
}

// ExitNode returns the exit node from the ExtractExitNode middleware.
func ExitNode(r *http.Request) database.ExitNode {
	node, ok := ExitNodeFromContext(r.Context())
	if !ok {
		panic("developer error: ExtractExitNode middleware not provided")
	}
	return node
}

// ExtractExitNode authenticates an exit node from the
// codersdk.ExitNodeTokenHeader header. The token format is
// "<exit node id>:<secret>", mirroring workspace proxy tokens. Every failure
// returns 401 so callers cannot distinguish a missing node from a bad secret
// beyond the detail string.
func ExtractExitNode(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			token := r.Header.Get(codersdk.ExitNodeTokenHeader)
			if token == "" {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Missing required exit node token",
				})
				return
			}

			idStr, secret, ok := strings.Cut(token, ":")
			if !ok {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid exit node token",
				})
				return
			}
			nodeID, err := uuid.Parse(idStr)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid exit node token",
				})
				return
			}
			if len(secret) != 64 {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid exit node token",
				})
				return
			}

			//nolint:gocritic // The exit node is looked up by ID to check its token.
			node, err := db.GetExitNodeByID(dbauthz.AsSystemRestricted(ctx), nodeID)
			if xerrors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid exit node token",
					Detail:  "Exit node not found.",
				})
				return
			}
			if err != nil {
				httpapi.InternalServerError(rw, err)
				return
			}
			if node.Deleted {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid exit node token",
					Detail:  "Exit node has been deleted.",
				})
				return
			}

			// Constant-time comparison of the hashed secret.
			if !apikey.ValidateHash(node.TokenHashedSecret, secret) {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid exit node token",
					Detail:  "Invalid exit node token secret.",
				})
				return
			}

			ctx = context.WithValue(ctx, exitNodeContextKey{}, node)
			//nolint:gocritic // Exit nodes act as a system component on the
			// few routes this middleware is mounted to, like workspace
			// proxies.
			ctx = dbauthz.AsSystemRestricted(ctx)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
