package runtimeconfig

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

var EntryNotFound = xerrors.New("entry not found")

type Store interface {
	GetRuntimeConfig(ctx context.Context, key string) (string, error)
	UpsertRuntimeConfig(ctx context.Context, arg database.UpsertRuntimeConfigParams) error
	DeleteRuntimeConfig(ctx context.Context, key string) error
}
