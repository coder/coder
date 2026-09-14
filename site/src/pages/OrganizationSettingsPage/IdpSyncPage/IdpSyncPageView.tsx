import type { FC } from "react";
import { useSearchParams } from "react-router";
import type {
	Group,
	GroupSyncSettings,
	Organization,
	Role,
	RoleSyncSettings,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import {
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";
import { ExportPolicyButton } from "#/modules/idpSync/ExportPolicyButton";
import { IdpGroupSyncForm } from "./IdpGroupSyncForm";
import { IdpRoleSyncForm } from "./IdpRoleSyncForm";

interface IdpSyncPageViewProps {
	tab: string;
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
	tab,
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
	const [_, setSearchParams] = useSearchParams();
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
			<Tabs
				value={tab}
				onValueChange={(value: string) => {
					setSearchParams({ tab: value });
				}}
			>
				<TabsList>
					<TabsTrigger value="groups">Group sync settings</TabsTrigger>
					<TabsTrigger value="roles">Role sync settings</TabsTrigger>
				</TabsList>
				<TabsContent value="groups" className="py-8">
					<div className="flex flex-col gap-6">
						<div className="flex justify-end">
							<ExportPolicyButton
								syncSettings={groupSyncSettings}
								filename={`${organization.name}_groups-policy.json`}
							/>
						</div>
						<IdpGroupSyncForm
							groupSyncSettings={groupSyncSettings}
							claimFieldValues={groupClaimFieldValues}
							groupMappingCount={groupMappingCount}
							legacyGroupMappingCount={legacyGroupMappingCount}
							groups={groups}
							groupsMap={groupsMap}
							onSubmit={onSubmitGroupSyncSettings}
							onSyncFieldChange={onGroupSyncFieldChange}
						/>
					</div>
				</TabsContent>
				<TabsContent value="roles" className="py-8">
					<div className="flex flex-col gap-6">
						<div className="flex justify-end">
							<ExportPolicyButton
								syncSettings={roleSyncSettings}
								filename={`${organization.name}_roles-policy.json`}
							/>
						</div>
						<IdpRoleSyncForm
							roleSyncSettings={roleSyncSettings}
							claimFieldValues={roleClaimFieldValues}
							roleMappingCount={roleMappingCount}
							roles={roles || []}
							onSubmit={onSubmitRoleSyncSettings}
							onSyncFieldChange={onRoleSyncFieldChange}
						/>
					</div>
				</TabsContent>
			</Tabs>
		</div>
	);
};

export default IdpSyncPageView;
