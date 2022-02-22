package parameter

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
)

type InjectOptions struct {
	ParameterSchemas []database.ParameterSchema
	ProvisionJobID   uuid.UUID

	Username   string
	Transition database.WorkspaceTransition
}

// Inject adds "coder*" parameters to a job.
func Inject(ctx context.Context, db database.Store, options InjectOptions) error {
	insertParameter := func(db database.Store, name, value string) error {
		_, err := db.InsertParameterValue(ctx, database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              name,
			CreatedAt:         database.Now(),
			UpdatedAt:         database.Now(),
			Scope:             database.ParameterScopeProvisionerJob,
			ScopeID:           options.ProvisionJobID.String(),
			SourceScheme:      database.ParameterSourceSchemeData,
			SourceValue:       value,
			DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
		})
		return err
	}
	return db.InTx(func(db database.Store) error {
		for _, schema := range options.ParameterSchemas {
			if !strings.HasPrefix(schema.Name, Namespace) {
				continue
			}
			var err error
			switch {
			case schema.Name == Username:
				err = insertParameter(db, schema.Name, options.Username)
			case schema.Name == WorkspaceTransition:
				// This is so provisionerd can inject it's own start/stop
				// for importing a project. This is not ideal.
				if options.Transition == "" {
					continue
				}
				err = insertParameter(db, schema.Name, string(options.Transition))
			case strings.HasPrefix(schema.Name, AgentTokenPrefix):
				// Generate an agent token here!
				err = insertParameter(db, schema.Name, uuid.NewString())
			default:
				return xerrors.Errorf("unrecognized namespaced parameter: %q", schema.Name)
			}
			if err != nil {
				return xerrors.Errorf("insert parameter %q: %w", schema.Name, err)
			}
		}
		return nil
	})
}
