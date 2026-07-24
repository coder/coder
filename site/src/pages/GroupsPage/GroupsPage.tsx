import { PlusIcon } from "lucide-react";
import { type FC, useEffect } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	groupsByOrganization,
	organizationGroupsAISpend,
} from "#/api/queries/groups";
import { organizationsPermissions } from "#/api/queries/organizations";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { useGroupsSettings } from "./GroupsPageProvider";
import { GroupsPageView, joinGroupsSpend } from "./GroupsPageView";

const GroupsPage: FC = () => {
	const { template_rbac: groupsEnabled, aibridge } = useFeatureVisibility();
	const { experiments } = useDashboard();
	const { organization, showOrganizations } = useGroupsSettings();
	// TODO(AIGOV-443): remove the ai-gateway-cost-control experiment gate once
	// the cost-control feature is stable.
	const aibridgeVisible =
		Boolean(aibridge) && experiments.includes("ai-gateway-cost-control");
	const groupsQuery = useQuery({
		...groupsByOrganization(organization?.name ?? ""),
		enabled: Boolean(organization),
	});
	const groupIds = groupsQuery.data?.map((group) => group.id) ?? [];
	const groupsSpendQuery = useQuery({
		...organizationGroupsAISpend(organization?.name ?? "", groupIds),
		enabled: aibridgeVisible && Boolean(organization) && groupIds.length > 0,
	});
	const groupsWithSpend = joinGroupsSpend(
		groupsQuery.data,
		groupsSpendQuery.data,
	);
	const permissionsQuery = useQuery({
		...organizationsPermissions([organization?.id ?? ""]),
		enabled: Boolean(organization),
	});

	useEffect(() => {
		if (groupsQuery.error) {
			toast.error(
				getErrorMessage(groupsQuery.error, "Unable to load groups."),
				{
					description: getErrorDetail(groupsQuery.error),
				},
			);
		}
	}, [groupsQuery.error]);

	useEffect(() => {
		if (groupsSpendQuery.error) {
			toast.error(
				getErrorMessage(groupsSpendQuery.error, "Unable to load AI spend."),
				{
					description: getErrorDetail(groupsSpendQuery.error),
				},
			);
		}
	}, [groupsSpendQuery.error]);

	useEffect(() => {
		if (permissionsQuery.error) {
			toast.error(
				getErrorMessage(permissionsQuery.error, "Unable to load permissions."),
				{
					description: getErrorDetail(permissionsQuery.error),
				},
			);
		}
	}, [permissionsQuery.error]);

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	if (permissionsQuery.isLoading) {
		return <Loader />;
	}

	const title = <title>{pageTitle("Groups")}</title>;

	const permissions = permissionsQuery.data?.[organization.id];

	if (!permissions?.viewGroups) {
		return (
			<>
				{title}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			{title}

			<div className="flex max-w-full flex-row items-baseline justify-between gap-4">
				<SettingsHeader>
					<SettingsHeaderTitle>Groups</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Manage groups for this{" "}
						{showOrganizations ? "organization" : "deployment"}.
					</SettingsHeaderDescription>
				</SettingsHeader>

				{groupsEnabled && permissions.createGroup && (
					<Button asChild>
						<RouterLink to="create">
							<PlusIcon className="size-icon-sm" />
							Create group
						</RouterLink>
					</Button>
				)}
			</div>

			<GroupsPageView
				groups={groupsWithSpend}
				spendError={groupsSpendQuery.isError}
				canCreateGroup={permissions.createGroup}
				groupsEnabled={groupsEnabled}
				showAIBudget={aibridgeVisible}
			/>
		</div>
	);
};

export default GroupsPage;
