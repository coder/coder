import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatUsageLimitConfig,
	deleteChatUsageLimitGroupOverride,
	deleteChatUsageLimitOverride,
	updateChatUsageLimitConfig,
	upsertChatUsageLimitGroupOverride,
	upsertChatUsageLimitOverride,
} from "#/api/queries/chats";
import { groups } from "#/api/queries/groups";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { LimitsTab } from "./components/LimitsTab";

const AgentSettingsLimitsPage: FC = () => {
	const { permissions } = useAuthenticated();

	const queryClient = useQueryClient();

	// Queries.
	const configQuery = useQuery(chatUsageLimitConfig());
	const groupsQuery = useQuery(groups());

	// Mutations.
	const updateConfigMutation = useMutation(
		updateChatUsageLimitConfig(queryClient),
	);
	const upsertOverrideMutation = useMutation(
		upsertChatUsageLimitOverride(queryClient),
	);
	const deleteOverrideMutation = useMutation(
		deleteChatUsageLimitOverride(queryClient),
	);
	const upsertGroupOverrideMutation = useMutation(
		upsertChatUsageLimitGroupOverride(queryClient),
	);
	const deleteGroupOverrideMutation = useMutation(
		deleteChatUsageLimitGroupOverride(queryClient),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<LimitsTab
				configData={configQuery.data}
				isLoadingConfig={configQuery.isLoading}
				configError={configQuery.isError ? configQuery.error : null}
				refetchConfig={() => void configQuery.refetch()}
				groupsData={groupsQuery.data}
				isLoadingGroups={groupsQuery.isLoading}
				groupsError={groupsQuery.isError ? groupsQuery.error : null}
				onUpdateConfig={(req) => updateConfigMutation.mutateAsync(req)}
				isUpdatingConfig={updateConfigMutation.isPending}
				updateConfigError={
					updateConfigMutation.isError ? updateConfigMutation.error : null
				}
				isUpdateConfigSuccess={updateConfigMutation.isSuccess}
				resetUpdateConfig={() => updateConfigMutation.reset()}
				onUpsertOverride={(args) => upsertOverrideMutation.mutateAsync(args)}
				isUpsertingOverride={upsertOverrideMutation.isPending}
				upsertOverrideError={
					upsertOverrideMutation.isError ? upsertOverrideMutation.error : null
				}
				onDeleteOverride={(userID) =>
					deleteOverrideMutation.mutateAsync(userID)
				}
				isDeletingOverride={deleteOverrideMutation.isPending}
				deleteOverrideError={
					deleteOverrideMutation.isError ? deleteOverrideMutation.error : null
				}
				onUpsertGroupOverride={(args) =>
					upsertGroupOverrideMutation.mutateAsync(args)
				}
				isUpsertingGroupOverride={upsertGroupOverrideMutation.isPending}
				upsertGroupOverrideError={
					upsertGroupOverrideMutation.isError
						? upsertGroupOverrideMutation.error
						: null
				}
				onDeleteGroupOverride={(groupID) =>
					deleteGroupOverrideMutation.mutateAsync(groupID)
				}
				isDeletingGroupOverride={deleteGroupOverrideMutation.isPending}
				deleteGroupOverrideError={
					deleteGroupOverrideMutation.isError
						? deleteGroupOverrideMutation.error
						: null
				}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsLimitsPage;
