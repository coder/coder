package healthcheck

import (
	"context"
	"sort"
	"time"

	"golang.org/x/mod/semver"

	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

// @typescript-generate ProvisionerDaemonsReport
type ProvisionerDaemonsReport struct {
	Severity  health.Severity  `json:"severity"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`
	Error     *string          `json:"error"`

	Items []ProvisionerDaemonsReportItem `json:"items"`
}

// @typescript-generate ProvisionerDaemonsReportItem
type ProvisionerDaemonsReportItem struct {
	codersdk.ProvisionerDaemon `json:"provisioner_daemon"`
	Warnings                   []health.Message `json:"warnings"`
}

type ProvisionerDaemonsReportDeps struct {
	// Required
	CurrentVersion         string
	CurrentAPIMajorVersion int
	Store                  ProvisionerDaemonsStore

	// Optional
	TimeNow       func() time.Time // Defaults to dbtime.Now
	StaleInterval time.Duration    // Defaults to 3 heartbeats

	Dismissed bool
}

type ProvisionerDaemonsStore interface {
	GetProvisionerDaemons(ctx context.Context) ([]database.ProvisionerDaemon, error)
}

func (r *ProvisionerDaemonsReport) Run(ctx context.Context, opts *ProvisionerDaemonsReportDeps) {
	r.Items = make([]ProvisionerDaemonsReportItem, 0)
	r.Severity = health.SeverityOK
	r.Warnings = make([]health.Message, 0)
	r.Dismissed = opts.Dismissed

	if opts.TimeNow == nil {
		opts.TimeNow = dbtime.Now
	}
	now := opts.TimeNow()

	if opts.StaleInterval == 0 {
		opts.StaleInterval = provisionerdserver.DefaultHeartbeatInterval * 3
	}

	if opts.CurrentVersion == "" {
		r.Severity = health.SeverityError
		r.Error = ptr.Ref("Developer error: CurrentVersion is empty!")
		return
	}

	if opts.CurrentAPIMajorVersion == 0 {
		r.Severity = health.SeverityError
		r.Error = ptr.Ref("Developer error: CurrentAPIMajorVersion must be non-zero!")
		return
	}

	if opts.Store == nil {
		r.Severity = health.SeverityError
		r.Error = ptr.Ref("Developer error: Store is nil!")
		return
	}

	// nolint: gocritic // need an actor to fetch provisioner daemons
	daemons, err := opts.Store.GetProvisionerDaemons(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		r.Severity = health.SeverityError
		r.Error = ptr.Ref("error fetching provisioner daemons: " + err.Error())
		return
	}

	// Ensure stable order for display and for tests
	sort.Slice(daemons, func(i, j int) bool {
		return daemons[i].Name < daemons[j].Name
	})

	for _, daemon := range daemons {
		// Daemon never connected, skip.
		if !daemon.LastSeenAt.Valid {
			continue
		}
		// Daemon has gone away, skip.
		if now.Sub(daemon.LastSeenAt.Time) > (opts.StaleInterval) {
			continue
		}

		it := ProvisionerDaemonsReportItem{
			ProvisionerDaemon: db2sdk.ProvisionerDaemon(daemon),
			Warnings:          make([]health.Message, 0),
		}

		// For release versions, just check MAJOR.MINOR and ignore patch.
		if !semver.IsValid(daemon.Version) {
			if r.Severity.Value() < health.SeverityError.Value() {
				r.Severity = health.SeverityError
			}
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Some provisioner daemons report invalid version information."))
			it.Warnings = append(it.Warnings, health.Messagef(health.CodeUnknown, "Invalid version %q", daemon.Version))
		} else if !buildinfo.VersionsMatch(opts.CurrentVersion, daemon.Version) {
			if r.Severity.Value() < health.SeverityWarning.Value() {
				r.Severity = health.SeverityWarning
			}
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeProvisionerDaemonVersionMismatch, "Some provisioner daemons report mismatched versions."))
			it.Warnings = append(it.Warnings, health.Messagef(health.CodeProvisionerDaemonVersionMismatch, "Mismatched version %q", daemon.Version))
		}

		// Provisioner daemon API version follows different rules; we just want to check the major API version and
		// warn about potential later deprecations.
		// When we check API versions of connecting provisioner daemons, all active provisioner daemons
		// will, by necessity, have a compatible API version.
		if maj, _, err := apiversion.Parse(daemon.APIVersion); err != nil {
			if r.Severity.Value() < health.SeverityError.Value() {
				r.Severity = health.SeverityError
			}
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Some provisioner daemons report invalid API version information."))
			it.Warnings = append(it.Warnings, health.Messagef(health.CodeUnknown, "Invalid API version: %s", err.Error())) // contains version string
		} else if maj != opts.CurrentAPIMajorVersion {
			if r.Severity.Value() < health.SeverityWarning.Value() {
				r.Severity = health.SeverityWarning
			}
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeProvisionerDaemonAPIMajorVersionDeprecated, "Some provisioner daemons report deprecated major API versions. Consider upgrading!"))
			it.Warnings = append(it.Warnings, health.Messagef(health.CodeProvisionerDaemonAPIMajorVersionDeprecated, "Deprecated major API version %d.", provisionersdk.CurrentMajor))
		}

		r.Items = append(r.Items, it)
	}

	if len(r.Items) == 0 {
		r.Severity = health.SeverityError
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeProvisionerDaemonsNoProvisionerDaemons, "No active provisioner daemons found!"))
		return
	}
}
