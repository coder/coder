package httpmw

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
)

// BumpWorkspaceAutoStop automatically bumps the workspace's auto-off timer
// if it is set to expire soon.
// It must be ran after ExtractWorkspace.
func BumpWorkspaceAutoStop(log slog.Logger, db database.Store) func(h http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspace := WorkspaceParam(r)

			err := db.InTx(func(s database.Store) error {
				build, err := s.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
				if errors.Is(err, sql.ErrNoRows) {
					return nil
				} else if err != nil {
					return xerrors.Errorf("get latest workspace build: %w", err)
				}

				job, err := s.GetProvisionerJobByID(r.Context(), build.JobID)
				if err != nil {
					return xerrors.Errorf("get provisioner job: %w", err)
				}

				if build.Transition != database.WorkspaceTransitionStart || !job.CompletedAt.Valid {
					return nil
				}

				if build.Deadline.IsZero() {
					// Workspace shutdown is manual
					return nil
				}

				// We sent bumpThreshold slightly under bumpAmount to minimize DB writes.
				const (
					bumpAmount    = time.Hour
					bumpThreshold = time.Hour - time.Minute*10
				)

				if !build.Deadline.Before(time.Now().Add(bumpThreshold)) {
					return nil
				}

				newDeadline := time.Now().Add(bumpAmount)

				if err := s.UpdateWorkspaceBuildByID(r.Context(), database.UpdateWorkspaceBuildByIDParams{
					ID:               build.ID,
					UpdatedAt:        build.UpdatedAt,
					ProvisionerState: build.ProvisionerState,
					Deadline:         newDeadline,
				}); err != nil {
					return xerrors.Errorf("update workspace build: %w", err)
				}
				return nil
			})

			if err != nil {
				log.Error(r.Context(), "auto-bump", slog.Error(err))
			}

			next.ServeHTTP(w, r)
		})
	}
}
