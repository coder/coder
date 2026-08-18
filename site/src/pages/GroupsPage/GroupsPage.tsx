import { type FC, useEffect } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	organizationGroupsAISpend,
	paginatedGroupsByOrganization,
} from "#/api/queries/groups";
import { organizationsPermissions } from "#/api/queries/organizations";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { useFilter } from "#/components/Filter/Filter";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { useGroupsSettings } from "./GroupsPageProvider";
import { GroupsPageView, joinGroupsSpend } from "./GroupsPageView";

const GroupsPage: FC = () => {
	const { permissions: authPermissions } = useAuthenticated();
	const { template_rbac: groupsEnabled, aibridge } = useFeatureVisibility();
	const { organization, showOrganizations } = useGroupsSettings();
	const aibridgeVisible = Boolean(aibridge);
	const [searchParams, setSearchParams] = useSearchParams();
	const groupsQuery = usePaginatedQuery({
		...paginatedGroupsByOrganization(organization?.name ?? "", searchParams),
		// Skip the request when unentitled; the paywall covers it. Boolean() is
		// required since groupsEnabled is undefined (not false) when unlicensed,
		// and React Query treats enabled: undefined as enabled.
		enabled: Boolean(groupsEnabled && organization),
	});
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: groupsQuery.goToFirstPage,
	});
	const groupIds = groupsQuery.data?.groups.map((group) => group.id) ?? [];
	const groupsSpendQuery = useQuery({
		...organizationGroupsAISpend(organization?.name ?? "", groupIds),
		enabled: aibridgeVisible && Boolean(organization) && groupIds.length > 0,
	});
	const groupsWithSpend = joinGroupsSpend(
		groupsQuery.data?.groups,
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
			</div>

			<GroupsPageView
				groups={groupsWithSpend}
				spendError={groupsSpendQuery.isError}
				canCreateGroup={permissions.createGroup}
				groupsEnabled={groupsEnabled}
				showAIBudget={aibridgeVisible}
				filterProps={{ filter }}
				groupsQuery={groupsQuery}
				permissions={authPermissions}
			/>
		</div>
	);
};

export default GroupsPage;
