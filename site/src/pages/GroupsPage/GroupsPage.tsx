import { PlusIcon } from "lucide-react";
import { type FC, useEffect, useMemo } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { groupsByOrganization } from "#/api/queries/groups";
import { organizationsPermissions } from "#/api/queries/organizations";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { useGroupsSettings } from "./GroupsPageProvider";
import type { GroupBudgetInfo } from "./GroupsPageView";
import { GroupsPageView } from "./GroupsPageView";

// Fixed set of mock budgets cycled across groups. Two of the five
// entries are over 90% so they render in red.
const MOCK_BUDGETS: GroupBudgetInfo[] = [
	{ spentUSD: 25492, limitUSD: null, aiSeats: 2 },
	{ spentUSD: 174978, limitUSD: 175000, aiSeats: 36 },  // >90%
	{ spentUSD: 32211, limitUSD: 50000, aiSeats: 0 },
	{ spentUSD: 71200, limitUSD: 75000, aiSeats: 14 },    // >90%
	{ spentUSD: 110345, limitUSD: 127000, aiSeats: 27 },
];

const GroupsPage: FC = () => {
	const { template_rbac: groupsEnabled } = useFeatureVisibility();
	const { organization, showOrganizations } = useGroupsSettings();
	const groupsQuery = useQuery({
		...groupsByOrganization(organization?.name ?? ""),
		enabled: Boolean(organization),
	});
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
		if (permissionsQuery.error) {
			toast.error(
				getErrorMessage(permissionsQuery.error, "Unable to load permissions."),
				{
					description: getErrorDetail(permissionsQuery.error),
				},
			);
		}
	}, [permissionsQuery.error]);

	// Build mock budget data from whichever groups the API returns.
	const budgets = useMemo(() => {
		if (!groupsQuery.data) return undefined;
		const map: Record<string, GroupBudgetInfo> = {};
		for (let i = 0; i < groupsQuery.data.length; i++) {
			map[groupsQuery.data[i].id] =
				MOCK_BUDGETS[i % MOCK_BUDGETS.length];
		}
		return map;
	}, [groupsQuery.data]);

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
						Manage groups and access for the{" "}
						<strong>{organization.display_name || organization.name}</strong>{" "}
						organization.
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
				groups={groupsQuery.data}
				canCreateGroup={permissions.createGroup}
				groupsEnabled={groupsEnabled}
				budgets={budgets}
			/>
		</div>
	);
};

export default GroupsPage;
