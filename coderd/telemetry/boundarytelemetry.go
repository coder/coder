package telemetry

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// BoundaryTelemetryCollector collects boundary feature usage data in memory
// and periodically flushes it to the database for telemetry reporting.
type BoundaryTelemetryCollector struct {
	db     database.Store
	logger slog.Logger

	mu               sync.Mutex
	activeUsers      map[uuid.UUID]struct{}
	activeWorkspaces map[uuid.UUID]uuid.UUID // workspaceID -> templateID
}

// NewBoundaryTelemetryCollector creates a new collector for boundary telemetry.
func NewBoundaryTelemetryCollector(db database.Store, logger slog.Logger) *BoundaryTelemetryCollector {
	return &BoundaryTelemetryCollector{
		db:               db,
		logger:           logger,
		activeUsers:      make(map[uuid.UUID]struct{}),
		activeWorkspaces: make(map[uuid.UUID]uuid.UUID),
	}
}

// RecordBoundaryUsage records that a user/workspace used the boundary feature.
func (c *BoundaryTelemetryCollector) RecordBoundaryUsage(userID, workspaceID, templateID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.activeUsers[userID] = struct{}{}
	c.activeWorkspaces[workspaceID] = templateID
}

// Flush writes the collected data to the database and clears the in-memory state.
// This should be called on the same interval as the telemetry snapshot.
func (c *BoundaryTelemetryCollector) Flush(ctx context.Context) error {
	c.mu.Lock()
	users := c.activeUsers
	workspaces := c.activeWorkspaces
	c.activeUsers = make(map[uuid.UUID]struct{})
	c.activeWorkspaces = make(map[uuid.UUID]uuid.UUID)
	c.mu.Unlock()

	now := dbtime.Now()

	for userID := range users {
		err := c.db.InsertBoundaryActiveUser(ctx, database.InsertBoundaryActiveUserParams{
			UserID:     userID,
			RecordedAt: now,
		})
		if err != nil {
			c.logger.Error(ctx, "failed to insert boundary active user",
				slog.F("user_id", userID),
				slog.Error(err))
		}
	}

	for workspaceID, templateID := range workspaces {
		err := c.db.InsertBoundaryActiveWorkspace(ctx, database.InsertBoundaryActiveWorkspaceParams{
			WorkspaceID: workspaceID,
			TemplateID:  templateID,
			RecordedAt:  now,
		})
		if err != nil {
			c.logger.Error(ctx, "failed to insert boundary active workspace",
				slog.F("workspace_id", workspaceID),
				slog.F("template_id", templateID),
				slog.Error(err))
		}
	}

	return nil
}

// Cleanup removes old boundary telemetry data from the database.
func (c *BoundaryTelemetryCollector) Cleanup(ctx context.Context, before time.Time) error {
	err := c.db.DeleteBoundaryActiveUsersBefore(ctx, before)
	if err != nil {
		return err
	}
	return c.db.DeleteBoundaryActiveWorkspacesBefore(ctx, before)
}
