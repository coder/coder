package healthcheck

import (
	"context"
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
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
)

type ProvisionerDaemonsReport healthsdk.ProvisionerDaemonsReport

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
	r.Items = make([]healthsdk.ProvisionerDaemonsReportItem, 0)
	r.Severity = health.SeverityOK
	r.Warnings = make([]health.Message, 0)
	r.Dismissed = opts.Dismissed

	if opts.TimeNow == nil {
		opts.TimeNow = dbtime.Now
	}
	now := opts.TimeNow()

	if opts.StaleInterval == 0 {
		opts.StaleInterval = provisionerdserver.StaleInterval
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

	recentDaemons := db2sdk.RecentProvisionerDaemons(now, opts.StaleInterval, daemons)
	for _, daemon := range recentDaemons {
		it := healthsdk.ProvisionerDaemonsReportItem{
			ProvisionerDaemon: daemon,
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
			it.Warnings = append(it.Warnings, health.Messagef(health.CodeProvisionerDaemonAPIMajorVersionDeprecated, "Deprecated major API version %d.", proto.CurrentMajor))
		}

		r.Items = append(r.Items, it)
	}

	if len(r.Items) == 0 {
		r.Severity = health.SeverityError
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeProvisionerDaemonsNoProvisionerDaemons, "No active provisioner daemons found!"))
		return
	}
}
