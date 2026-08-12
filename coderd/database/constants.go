package database

import (
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// PrebuildsSystemUserID mirrors codersdk.PrebuildsSystemUserID, parsed
// for use as a uuid.UUID. Both must agree; tests pin the value to the
// codersdk constant so the two cannot drift.
var PrebuildsSystemUserID = uuid.MustParse(codersdk.PrebuildsSystemUserID)
