// Package sdk2db provides common conversion routines from codersdk types to database types
package sdk2db

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

func ProvisionerDaemonStatus(status codersdk.ProvisionerDaemonStatus) database.ProvisionerDaemonStatus {
	return database.ProvisionerDaemonStatus(status)
}

func ProvisionerDaemonStatuses(params []codersdk.ProvisionerDaemonStatus) []database.ProvisionerDaemonStatus {
	return slice.List(params, ProvisionerDaemonStatus)
}
