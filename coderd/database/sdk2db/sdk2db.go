// Package sdk2db provides common conversion routines from codersdk types to database types
package sdk2db

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/codersdk"
)

func ProvisionerDaemonStatus(status codersdk.ProvisionerDaemonStatus) database.ProvisionerDaemonStatus {
	return database.ProvisionerDaemonStatus(status)
}

func ProvisionerDaemonStatuses(params []codersdk.ProvisionerDaemonStatus) []database.ProvisionerDaemonStatus {
	return db2sdk.List(params, ProvisionerDaemonStatus)
}
