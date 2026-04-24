package database

import "github.com/google/uuid"

var PrebuildsSystemUserID = uuid.MustParse("c42fdf75-3097-471c-8c33-fb52454d81c0")

// DefaultChatAutoArchiveDays is the default value passed to
// GetChatAutoArchiveDays when no site config row exists. A value of 0
// disables auto-archival.
const DefaultChatAutoArchiveDays int32 = 0
