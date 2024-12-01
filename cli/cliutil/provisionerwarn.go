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

func WarnMatchedProvisioners(w io.Writer, tv codersdk.TemplateVersion) {
	if tv.MatchedProvisioners == nil {
		// Nothing in the response, nothing to do here!
		return
	}
	var tagsJSON strings.Builder
	if err := json.NewEncoder(&tagsJSON).Encode(tv.Job.Tags); err != nil {
		// Fall back to the less-pretty string representation.
		tagsJSON.Reset()
		_, _ = tagsJSON.WriteString(fmt.Sprintf("%v", tv.Job.Tags))
	}
	if tv.MatchedProvisioners.Count == 0 {
		cliui.Warnf(w, warnNoMatchedProvisioners, tv.Job.ID, tagsJSON.String())
		return
	}
	if tv.MatchedProvisioners.Available == 0 {
		cliui.Warnf(w, warnNoAvailableProvisioners, tv.Job.ID, strings.TrimSpace(tagsJSON.String()), tv.MatchedProvisioners.MostRecentlySeen.Time)
		return
	}
}
