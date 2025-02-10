import type {
	Group,
	GroupSyncSettings,
	Organization,
	Role,
	RoleSyncSettings,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import type { FC } from "react";
import { IdpGroupSyncForm } from "./IdpGroupSyncForm";
import { IdpRoleSyncForm } from "./IdpRoleSyncForm";

interface IdpSyncPageViewProps {
	tab: string;
	groupSyncSettings: GroupSyncSettings | undefined;
	roleSyncSettings: RoleSyncSettings | undefined;
	claimFieldValues: readonly string[] | undefined;
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

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({
	tab,
	groupSyncSettings,
	roleSyncSettings,
	claimFieldValues,
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
		<div className="flex flex-col gap-4">
			{Boolean(error) && <ErrorAlert error={error} />}
			<Tabs active={tab}>
				<TabsList>
					<TabLink to="?tab=groups" value="groups">
						Group sync settings
					</TabLink>
					<TabLink to="?tab=roles" value="roles">
						Role sync settings
					</TabLink>
				</TabsList>
			</Tabs>
			{tab === "groups" ? (
				<IdpGroupSyncForm
					groupSyncSettings={groupSyncSettings}
					claimFieldValues={claimFieldValues}
					groupMappingCount={groupMappingCount}
					legacyGroupMappingCount={legacyGroupMappingCount}
					groups={groups}
					groupsMap={groupsMap}
					organization={organization}
					onSubmit={onSubmitGroupSyncSettings}
					onSyncFieldChange={onGroupSyncFieldChange}
				/>
			) : (
				<IdpRoleSyncForm
					roleSyncSettings={roleSyncSettings}
					claimFieldValues={claimFieldValues}
					roleMappingCount={roleMappingCount}
					roles={roles || []}
					organization={organization}
					onSubmit={onSubmitRoleSyncSettings}
					onSyncFieldChange={onRoleSyncFieldChange}
				/>
			)}
		</div>
	);
};

export default IdpSyncPageView;
