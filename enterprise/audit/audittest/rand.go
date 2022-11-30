package audittest

import (
	"database/sql"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tabbed/pqtype"

	"github.com/coder/coder/coderd/database"
)

func RandomLog() database.AuditLog {
	_, inet, _ := net.ParseCIDR("127.0.0.1/32")
	return database.AuditLog{
		ID:             uuid.New(),
		Time:           time.Now(),
		UserID:         uuid.New(),
		OrganizationID: uuid.New(),
		Ip: pqtype.Inet{
			IPNet: *inet,
			Valid: true,
		},
		UserAgent:      sql.NullString{String: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36", Valid: true},
		ResourceType:   database.ResourceTypeOrganization,
		ResourceID:     uuid.New(),
		ResourceTarget: "colin's organization",
		Action:         database.AuditActionDelete,
		Diff:           []byte("{}"),
		StatusCode:     http.StatusNoContent,
	}
}
