package insightsapi

import (
	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpauthz"
)

type API struct {
	logger     slog.Logger
	authorizer *httpauthz.HTTPAuthorizer
	database   database.Store
}

func NewAPI(
	logger slog.Logger,
	db database.Store,
	authorizer *httpauthz.HTTPAuthorizer,
) *API {
	a := &API{
		logger:     logger.Named("insightsapi"),
		authorizer: authorizer,
		database:   db,
	}
	return a
}
