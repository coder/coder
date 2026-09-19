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
			unauthorized := func(message, detail string) {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{Message: message, Detail: detail})
			}

			token := r.Header.Get(codersdk.ExitNodeTokenHeader)
			if token == "" {
				unauthorized("Missing required exit node token", "")
				return
			}
			idStr, secret, ok := strings.Cut(token, ":")
			nodeID, err := uuid.Parse(idStr)
			if !ok || err != nil || len(secret) != 64 {
				unauthorized("Invalid exit node token", "")
				return
			}

			//nolint:gocritic // The exit node is looked up by ID to check its token.
			node, err := db.GetExitNodeByID(dbauthz.AsSystemRestricted(ctx), nodeID)
			switch {
			case xerrors.Is(err, sql.ErrNoRows):
				unauthorized("Invalid exit node token", "Exit node not found.")
				return
			case err != nil:
				httpapi.InternalServerError(rw, err)
				return
			case node.Deleted:
				unauthorized("Invalid exit node token", "Exit node has been deleted.")
				return
			// Constant-time comparison of the hashed secret.
			case !apikey.ValidateHash(node.TokenHashedSecret, secret):
				unauthorized("Invalid exit node token", "Invalid exit node token secret.")
				return
			}

			ctx = context.WithValue(ctx, exitNodeContextKey{}, node)
			//nolint:gocritic // Exit nodes act as a system component on the
			// few routes this middleware is mounted to, like workspace
			// proxies.
			next.ServeHTTP(rw, r.WithContext(dbauthz.AsSystemRestricted(ctx)))
		})
	}
}
