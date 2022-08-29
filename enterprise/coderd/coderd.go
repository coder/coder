package coderd

import (
	"context"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/rbac"
)

const EnvAuditLogEnable = "CODER_AUDIT_LOG_ENABLE"

func NewEnterprise(options *coderd.Options) *coderd.API {
	var eOpts = *options
	if eOpts.Authorizer == nil {
		var err error
		eOpts.Authorizer, err = rbac.NewAuthorizer()
		if err != nil {
			// This should never happen, as the unit tests would fail if the
			// default built in authorizer failed.
			panic(xerrors.Errorf("rego authorize panic: %w", err))
		}
	}
	eOpts.LicenseHandler = newLicenseAPI(
		eOpts.Logger,
		eOpts.Database,
		eOpts.Pubsub,
		&coderd.HTTPAuthorizer{
			Authorizer: eOpts.Authorizer,
			Logger:     eOpts.Logger,
		}).handler()
	en := Enablements{AuditLogs: true}
	auditLog := os.Getenv(EnvAuditLogEnable)
	auditLog = strings.ToLower(auditLog)
	if auditLog == "disable" || auditLog == "false" || auditLog == "0" || auditLog == "no" {
		en.AuditLogs = false
	}
	eOpts.FeaturesService = newFeaturesService(
		context.Background(),
		eOpts.Logger,
		eOpts.Database,
		eOpts.Pubsub,
		en,
	)
	return coderd.New(&eOpts)
}
