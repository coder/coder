package provisionerjobs

import (
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

const EventJobPosted = "provisioner_job_posted"

type JobPosting struct {
	ProvisionerType database.ProvisionerType `json:"type"`
	Tags            map[string]string        `json:"tags"`
}

func PostJob(ps pubsub.Pubsub, job database.ProvisionerJob) error {
	msg, err := json.Marshal(JobPosting{
		ProvisionerType: job.Provisioner,
		Tags:            job.Tags,
	})
	if err != nil {
		return xerrors.Errorf("marshal job posting: %w", err)
	}
	err = ps.Publish(EventJobPosted, msg)
	return err
}
