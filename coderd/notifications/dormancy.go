package notifications

import (
	"database/sql"

	"github.com/dustin/go-humanize"
)

// DormantDeletionText supplies the timeTilDormant label embedded in the stored
// TemplateWorkspaceDormant "will be automatically deleted in ..." sentence.
func DormantDeletionText(deletingAt sql.NullTime) string {
	if deletingAt.Valid {
		return humanize.Time(deletingAt.Time)
	}
	return "line with your template's auto-deletion policy"
}
