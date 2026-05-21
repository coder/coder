package provisionersdk

import (
	"fmt"

	"github.com/google/uuid"
)

// ProvisionerJobLogsNotifyMessage is the payload published on
// the provisioner job logs notify channel.
type ProvisionerJobLogsNotifyMessage struct {
	CreatedAfter int64 `json:"created_after"`
	EndOfLogs    bool  `json:"end_of_logs,omitempty"`
}

// ProvisionerJobLogsNotifyChannel is the PostgreSQL NOTIFY channel
// to publish updates to job logs on.
func ProvisionerJobLogsNotifyChannel(jobID uuid.UUID) string {
	return fmt.Sprintf("provisioner-log-logs:%s", jobID)
}
