package provisionerjobs
import (
	"fmt"
	"errors"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)
const EventJobPosted = "provisioner_job_posted"
type JobPosting struct {
	OrganizationID  uuid.UUID                `json:"organization_id"`
	ProvisionerType database.ProvisionerType `json:"type"`
	Tags            map[string]string        `json:"tags"`
}
func PostJob(ps pubsub.Pubsub, job database.ProvisionerJob) error {
	msg, err := json.Marshal(JobPosting{
		OrganizationID:  job.OrganizationID,
		ProvisionerType: job.Provisioner,
		Tags:            job.Tags,
	})
	if err != nil {
		return fmt.Errorf("marshal job posting: %w", err)
	}
	err = ps.Publish(EventJobPosted, msg)
	return err
}
