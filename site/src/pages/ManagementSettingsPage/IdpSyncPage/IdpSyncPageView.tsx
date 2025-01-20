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
import { useSearchParams } from "react-router-dom";
import { IdpGroupSyncForm } from "./IdpGroupSyncForm";
import { IdpRoleSyncForm } from "./IdpRoleSyncForm";

interface IdpSyncPageViewProps {
	groupSyncSettings: GroupSyncSettings | undefined;
	roleSyncSettings: RoleSyncSettings | undefined;
	groups: Group[] | undefined;
	groupsMap: Map<string, string>;
	roles: Role[] | undefined;
	organization: Organization;
	error?: unknown;
	onSubmitGroupSyncSettings: (data: GroupSyncSettings) => void;
	onSubmitRoleSyncSettings: (data: RoleSyncSettings) => void;
}

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({
	groupSyncSettings,
	roleSyncSettings,
	groups,
	groupsMap,
	roles,
	organization,
	error,
	onSubmitGroupSyncSettings,
	onSubmitRoleSyncSettings,
}) => {
	const [searchParams] = useSearchParams();
	const tab = searchParams.get("tab") || "groups";
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
					groupMappingCount={groupMappingCount}
					legacyGroupMappingCount={legacyGroupMappingCount}
					groups={groups}
					groupsMap={groupsMap}
					organization={organization}
					onSubmit={onSubmitGroupSyncSettings}
				/>
			) : (
				<IdpRoleSyncForm
					roleSyncSettings={roleSyncSettings}
					roleMappingCount={roleMappingCount}
					roles={roles || []}
					organization={organization}
					onSubmit={onSubmitRoleSyncSettings}
				/>
			)}
		</div>
	);
};

export default IdpSyncPageView;
