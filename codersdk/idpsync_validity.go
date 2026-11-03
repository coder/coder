package codersdk

// Finding codes for the validity conditions V2 through V5 in
// `coderd/idpsync/SYNC_SETTINGS.md`, in addition to the syntax codes. Validity
// depends on the state of the deployment, so only the server reports these
// codes, through ValidityFindings in `coderd/idpsync`. They appear in API
// warnings and rejections alongside the syntax codes.
const (
	// GroupSyncFindingOrganizationDeleted: the target organization is
	// soft-deleted (V2).
	GroupSyncFindingOrganizationDeleted GroupSyncFindingCode = "organization_deleted"
	// GroupSyncFindingOrganizationAbsent: no organization has the target ID
	// (V2).
	GroupSyncFindingOrganizationAbsent GroupSyncFindingCode = "organization_absent"
	// GroupSyncFindingMissingGroup: a `mapping` array contains a UUID that
	// names no group (V3).
	GroupSyncFindingMissingGroup GroupSyncFindingCode = "missing_group"
	// GroupSyncFindingOutsideGroup: a `mapping` array contains a UUID that
	// names a group in another organization (V4).
	GroupSyncFindingOutsideGroup GroupSyncFindingCode = "outside_group"
	// GroupSyncFindingEveryoneGroup: a `mapping` array contains the target
	// organization's Everyone group (V5).
	GroupSyncFindingEveryoneGroup GroupSyncFindingCode = "everyone_group"
)
