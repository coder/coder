import type { FC } from "react";
import type {
	Group,
	GroupSyncSettings,
	Organization,
	Role,
	RoleSyncSettings,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { IdpGroupSyncForm } from "./IdpGroupSyncForm";
import { IdpRoleSyncForm } from "./IdpRoleSyncForm";

interface IdpSyncPageViewProps {
	groupSyncSettings: GroupSyncSettings | undefined;
	roleSyncSettings: RoleSyncSettings | undefined;
	groupClaimFieldValues: readonly string[] | undefined;
	roleClaimFieldValues: readonly string[] | undefined;
	groups: Group[] | undefined;
	groupsMap: Map<string, string>;
	roles: Role[] | undefined;
	organization: Organization;
	onGroupSyncFieldChange: (value: string) => void;
	onRoleSyncFieldChange: (value: string) => void;
	error?: unknown;
	onSubmitGroupSyncSettings: (data: GroupSyncSettings) => void;
	onSubmitRoleSyncSettings: (data: RoleSyncSettings) => void;
}

const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({
	groupSyncSettings,
	roleSyncSettings,
	groupClaimFieldValues,
	roleClaimFieldValues,
	groups,
	groupsMap,
	roles,
	organization,
	onGroupSyncFieldChange,
	onRoleSyncFieldChange,
	error,
	onSubmitGroupSyncSettings,
	onSubmitRoleSyncSettings,
}) => {
	const groupMappingCount = groupSyncSettings?.mapping
		? Object.entries(groupSyncSettings.mapping).length
		: 0;
	const legacyGroupMappingCount = groupSyncSettings?.legacy_group_name_mapping
		? Object.entries(groupSyncSettings.legacy_group_name_mapping).length
		: 0;
	const roleMappingCount = roleSyncSettings?.mapping
		? Object.entries(roleSyncSettings.mapping).length
		: 0;

	if (!groupSyncSettings || !roleSyncSettings || !groups) {
		return <Loader />;
	}

	return (
		<div className="flex flex-col gap-12">
			{Boolean(error) && <ErrorAlert error={error} />}
			<IdpGroupSyncForm
				groupSyncSettings={groupSyncSettings}
				claimFieldValues={groupClaimFieldValues}
				groupMappingCount={groupMappingCount}
				legacyGroupMappingCount={legacyGroupMappingCount}
				groups={groups}
				groupsMap={groupsMap}
				organization={organization}
				onSubmit={onSubmitGroupSyncSettings}
				onSyncFieldChange={onGroupSyncFieldChange}
			/>
			<IdpRoleSyncForm
				roleSyncSettings={roleSyncSettings}
				claimFieldValues={roleClaimFieldValues}
				roleMappingCount={roleMappingCount}
				roles={roles || []}
				organization={organization}
				onSubmit={onSubmitRoleSyncSettings}
				onSyncFieldChange={onRoleSyncFieldChange}
			/>
		</div>
	);
};

export default IdpSyncPageView;
