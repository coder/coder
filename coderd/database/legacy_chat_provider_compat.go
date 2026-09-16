package database

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// ChatProvider is the fixture shape accepted by dbgen.ChatProvider.
//
//nolint:revive,staticcheck // Field names match the legacy generated database models.
type ChatProvider struct {
	ID                         uuid.UUID
	Provider                   string
	DisplayName                string
	APIKey                     string
	BaseUrl                    string
	ApiKeyKeyID                sql.NullString
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	CreatedBy                  uuid.NullUUID
	Enabled                    bool
	CentralApiKeyEnabled       bool
	AllowUserApiKey            bool
	AllowCentralApiKeyFallback bool
}

// InsertChatProviderParams is the callback parameter shape accepted by
// dbgen.ChatProvider.
//
//nolint:revive,staticcheck // Field names match the legacy generated database parameters.
type InsertChatProviderParams struct {
	Provider                   string
	DisplayName                string
	APIKey                     string
	BaseUrl                    string
	ApiKeyKeyID                sql.NullString
	CreatedBy                  uuid.NullUUID
	Enabled                    bool
	CentralApiKeyEnabled       bool
	AllowUserApiKey            bool
	AllowCentralApiKeyFallback bool
}
