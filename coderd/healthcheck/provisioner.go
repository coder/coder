package healthcheck

import (
	"context"

	"golang.org/x/mod/semver"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk"
)

// @typescript-generate ProvisionerDaemonReport
type ProvisionerDaemonReport struct {
	Severity  health.Severity  `json:"severity"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`
	Error     *string

	Provisioners []codersdk.ProvisionerDaemon
}

// @typescript-generate ProvisionerDaemonReportOptions
type ProvisionerDaemonReportOptions struct {
	CurrentVersion    string
	CurrentAPIVersion string

	// ProvisionerDaemonsFn is a function that returns ProvisionerDaemons.
	// Satisfied by database.Store.ProvisionerDaemons
	ProvisionerDaemonsFn func(context.Context) ([]database.ProvisionerDaemon, error)

	Dismissed bool
}

func (r *ProvisionerDaemonReport) Run(ctx context.Context, opts *ProvisionerDaemonReportOptions) {
	r.Severity = health.SeverityOK
	r.Warnings = make([]health.Message, 0)
	r.Dismissed = opts.Dismissed

	if opts.CurrentVersion == "" {
		r.Severity = health.SeverityError
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Developer error: CurrentVersion is empty!"))
		return
	}

	if opts.CurrentAPIVersion == "" {
		r.Severity = health.SeverityError
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Developer error: CurrentAPIVersion is empty!"))
		return
	}

	if opts.ProvisionerDaemonsFn == nil {
		r.Severity = health.SeverityError
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Developer error: ProvisionerDaemonsFn is nil!"))
		return
	}

	daemons, err := opts.ProvisionerDaemonsFn(ctx)
	if err != nil {
		r.Severity = health.SeverityError
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Unable to fetch provisioner daemons: %s", err.Error()))
		return
	}

	if len(daemons) == 0 {
		r.Severity = health.SeverityError
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeProvisionerDaemonsNoProvisionerDaemons, "No provisioner daemons found!"))
	}

	for _, daemon := range daemons {
		// For release versions, just check MAJOR.MINOR and ignore patch.
		if !semver.IsValid(daemon.Version) {
			r.Severity = health.SeverityWarning
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Provisioner daemon %q reports invalid version %q", opts.CurrentVersion, daemon.Version))
		} else if semver.Compare(semver.MajorMinor(opts.CurrentVersion), semver.MajorMinor(daemon.Version)) > 1 {
			r.Severity = health.SeverityWarning
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Provisioner daemon %q has outdated version %q", daemon.Name, daemon.Version))
		}

		// Provisioner daemon API version follows different rules.
		// 1) Coderd must support the requested API major version.
		// 2) The requested API minor version must be less than or equal to that of Coderd.
		ourMaj := semver.Major(opts.CurrentVersion)
		theirMaj := semver.Major(daemon.APIVersion)
		if semver.Compare(ourMaj, theirMaj) != 0 {
			r.Severity = health.SeverityError
			r.Warnings = append(r.Warnings, health.Messagef("Provisioner daemon %q requested major API version %s but only %s is available", daemon.Name, theirMaj, ourMaj))
		} else if semver.Compare(semver.MajorMinor(opts.CurrentAPIVersion), semver.MajorMinor(daemon.APIVersion)) > 1 {
			r.Severity = health.SeverityWarning
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeUnknown, "Provisioner daemon %q requested API version %q but only %q is available", daemon.Name, daemon.Version, opts.CurrentAPIVersion))
		}
	}
}
