package cliutil

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

var (
	warnNoMatchedProvisioners = `Your build has been enqueued, but there are no provisioners that accept the required tags. Once a compatible provisioner becomes available, your build will continue. Please contact your administrator.
Details:
	Provisioner job ID : %s
	Requested tags     : %s
`
	warnNoAvailableProvisioners = `Provisioners that accept the required tags have not responded for longer than expected. This may delay your build. Please contact your administrator if your build does not complete.
Details:
	Provisioner job ID : %s
	Requested tags     : %s
	Most recently seen : %s
`
)

// WarnMatchedProvisioners warns the user if there are no provisioners that
// match the requested tags for a given provisioner job.
// If the job is not pending, it is ignored.
func WarnMatchedProvisioners(w io.Writer, mp *codersdk.MatchedProvisioners, job codersdk.ProvisionerJob) {
	if mp == nil {
		// Nothing in the response, nothing to do here!
		return
	}
	if job.Status != codersdk.ProvisionerJobPending {
		// Only warn if the job is pending.
		return
	}
	var tagsJSON strings.Builder
	if err := json.NewEncoder(&tagsJSON).Encode(job.Tags); err != nil {
		// Fall back to the less-pretty string representation.
		tagsJSON.Reset()
		_, _ = tagsJSON.WriteString(fmt.Sprintf("%v", job.Tags))
	}
	if mp.Count == 0 {
		cliui.Warnf(w, warnNoMatchedProvisioners, job.ID, tagsJSON.String())
		return
	}
	if mp.Available == 0 {
		cliui.Warnf(w, warnNoAvailableProvisioners, job.ID, strings.TrimSpace(tagsJSON.String()), mp.MostRecentlySeen.Time)
		return
	}
}
