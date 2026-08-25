package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

type workspaceAgentContextKey struct{}

func WorkspaceAgentOptional(r *http.Request) (database.WorkspaceAgent, bool) {
	user, ok := r.Context().Value(workspaceAgentContextKey{}).(database.WorkspaceAgent)
	return user, ok
}

// WorkspaceAgent returns the workspace agent from the ExtractAgent handler.
func WorkspaceAgent(r *http.Request) database.WorkspaceAgent {
	user, ok := WorkspaceAgentOptional(r)
	if !ok {
		panic("developer error: agent middleware not provided or was made optional")
	}
	return user
}

type latestBuildContextKey struct{}

func latestBuildOptional(r *http.Request) (database.WorkspaceBuild, bool) {
	wb, ok := r.Context().Value(latestBuildContextKey{}).(database.WorkspaceBuild)
	return wb, ok
}

// LatestBuild returns the Latest Build from the ExtractLatestBuild handler.
func LatestBuild(r *http.Request) database.WorkspaceBuild {
	wb, ok := latestBuildOptional(r)
	if !ok {
		panic("developer error: agent middleware not provided or was made optional")
	}
	return wb
}

type ExtractWorkspaceAgentAndLatestBuildConfig struct {
	DB database.Store
	// Optional indicates whether the middleware should be optional.  If true, any
	// requests without the a token or with an invalid token will be allowed to
	// continue and no workspace agent will be set on the request context.
	Optional bool
}

// ExtractWorkspaceAgentAndLatestBuild requires authentication using a valid agent token.
func ExtractWorkspaceAgentAndLatestBuild(opts ExtractWorkspaceAgentAndLatestBuildConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// optionalWrite wraps httpapi.Write but runs the next handler if the
			// token is optional.
			//
			// It should be used when the token is not provided or is invalid, but not
			// when there are other errors.
			optionalWrite := func(code int, response codersdk.Response) {
				if opts.Optional {
					next.ServeHTTP(rw, r)
					return
				}
				httpapi.Write(ctx, rw, code, response)
			}

			tokenValue := APITokenFromRequest(r)
			if tokenValue == "" {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("Cookie %q must be provided.", codersdk.SessionTokenCookie),
				})
				return
			}
			token, err := uuid.Parse(tokenValue)
			if err != nil {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: "Workspace agent token invalid.",
					Detail:  fmt.Sprintf("An agent token must be a valid UUIDv4. (len %d)", len(tokenValue)),
				})
				return
			}

			//nolint:gocritic // System needs to be able to get workspace agents.
			row, err := opts.DB.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), token)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					optionalWrite(http.StatusUnauthorized, codersdk.Response{
						Message: "Workspace agent not authorized.",
						Detail:  "The agent cannot authenticate until the workspace provision job has been completed. If the job is no longer running, this agent is invalid.",
					})
					return
				}

				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error checking workspace agent authorization.",
					Detail:  err.Error(),
				})
				return
			}

			subjectUserID := row.WorkspaceTable.OwnerID
			blockUserData := row.WorkspaceAgent.APIKeyScope == database.AgentKeyScopeEnumNoUserData
			var identity *aiagentidentity.ResolvedIdentity
			if row.WorkspaceAgent.AIAgentID.Valid {
				if row.WorkspaceAgent.AIAgentID.UUID == uuid.Nil {
					httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
						Message: "Workspace agent not authorized.",
						Detail:  "AI agent identity is invalid or has been revoked.",
					})
					return
				}

				resolved, resolveErr := aiagentidentity.Resolve(ctx, opts.DB, row.WorkspaceAgent.AIAgentID.UUID)
				if resolveErr != nil {
					if errors.Is(resolveErr, aiagentidentity.ErrNotAIAgent) ||
						errors.Is(resolveErr, aiagentidentity.ErrAIAgentDeleted) ||
						errors.Is(resolveErr, sql.ErrNoRows) {
						httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
							Message: "Workspace agent not authorized.",
							Detail:  "AI agent identity is invalid or has been revoked.",
						})
						return
					}
					httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
						Message: "Internal error resolving workspace agent identity.",
						Detail:  resolveErr.Error(),
					})
					return
				}
				// Whether the agent is live is the ledger's answer and Resolve
				// has already given it, refusing anything but active. A second
				// check against a mirrored users row would be a second opinion
				// able to disagree with the authority.
				if resolved.OwnerUser.Deleted || resolved.OwnerUser.Status != database.UserStatusActive {
					httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
						Message: "Workspace agent not authorized.",
						Detail:  "AI agent owner is not active.",
					})
					return
				}
				subjectUserID = resolved.OwnerUser.ID
				blockUserData = true
				identity = &resolved
			}

			subject, userStatus, err := UserRBACSubject(
				ctx,
				opts.DB,
				subjectUserID,
				rbac.WorkspaceAgentScope(rbac.WorkspaceAgentScopeParams{
					WorkspaceID:   row.WorkspaceTable.ID,
					OwnerID:       row.WorkspaceTable.OwnerID,
					TemplateID:    row.WorkspaceTable.TemplateID,
					VersionID:     row.WorkspaceBuild.TemplateVersionID,
					TaskID:        row.TaskID,
					BlockUserData: blockUserData,
				}),
			)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error with workspace agent authorization context.",
					Detail:  err.Error(),
				})
				return
			}
			if identity != nil {
				if userStatus != database.UserStatusActive {
					httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
						Message: "Workspace agent not authorized.",
						Detail:  "AI agent owner is not active.",
					})
					return
				}
				// AsAIAgent carries the acting identity into the policy input so
				// a bound workspace agent is confined to workspaces designated
				// to its identity.
				subject = subject.AsAIAgent(identity.Ledger.ID, identity.Name())
				ctx = aiagentidentity.WithActor(ctx, identity.Actor)
			}

			ctx = context.WithValue(ctx, workspaceAgentContextKey{}, row.WorkspaceAgent)
			ctx = context.WithValue(ctx, latestBuildContextKey{}, row.WorkspaceBuild)
			// Also set the dbauthz actor for the request.
			ctx = dbauthz.As(ctx, subject)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
