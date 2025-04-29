import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import { getErrorMessage } from "api/errors";
import { groupsByOrganization } from "api/queries/groups";
import { organizationsPermissions } from "api/queries/organizations";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "modules/permissions/RequirePermission";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useGroupsSettings } from "./GroupsPageProvider";
import GroupsPageView from "./GroupsPageView";

const GroupsPage: FC = () => {
	const { template_rbac: groupsEnabled } = useFeatureVisibility();
	const { organization, showOrganizations } = useGroupsSettings();
	const groupsQuery = useQuery(
		organization ? groupsByOrganization(organization.name) : { enabled: false },
	);
	const permissionsQuery = useQuery(
		organization
			? organizationsPermissions([organization.id])
			: { enabled: false },
	);

	useEffect(() => {
		if (groupsQuery.error) {
			displayError(
				getErrorMessage(groupsQuery.error, "Unable to load groups."),
			);
		}
	}, [groupsQuery.error]);

	useEffect(() => {
		if (permissionsQuery.error) {
			displayError(
				getErrorMessage(permissionsQuery.error, "Unable to load permissions."),
			);
		}
	}, [permissionsQuery.error]);

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	if (permissionsQuery.isLoading) {
		return <Loader />;
	}

	const helmet = (
		<Helmet>
			<title>{pageTitle("Groups")}</title>
		</Helmet>
	);

	const permissions = permissionsQuery.data?.[organization.id];

	if (!permissions?.viewGroups) {
		return (
			<>
				{helmet}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	return (
		<>
			{helmet}

			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
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
							<GroupAdd />
							Create group
						</RouterLink>
					</Button>
				)}
			</Stack>

			<GroupsPageView
				groups={groupsQuery.data}
				canCreateGroup={permissions.createGroup}
				groupsEnabled={groupsEnabled}
			/>
		</>
	);
};

export default GroupsPage;
