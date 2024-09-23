import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import Button from "@mui/material/Button";
import { getErrorMessage } from "api/errors";
import { groupsByOrganization } from "api/queries/groups";
import { organizationPermissions } from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import GroupsPageView from "./GroupsPageView";
import { Breadcrumbs, Crumb } from "components/Breadcrumbs/Breadcrumbs";

export const GroupsPage: FC = () => {
	const feats = useFeatureVisibility();
	const { organization } = useOrganizationSettings();
	const groupsQuery = useQuery(
		organization ? groupsByOrganization(organization.name) : { enabled: false },
	);
	const permissionsQuery = useQuery(organizationPermissions(organization?.id));

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

	const permissions = permissionsQuery.data;
	if (!permissions) {
		return <Loader />;
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle("Groups")}</title>
			</Helmet>

			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
				css={{ paddingBottom: 32 }}
			>
				<Stack direction="row" spacing={2} alignItems="center">
					<Breadcrumbs>
						<Crumb>Organizations</Crumb>
						<Crumb href={`/organizations/${organization}`}>
							{organization.display_name || organization.name}
						</Crumb>
						<Crumb href={`/organizations/${organization}/groups`} active>
							Groups
						</Crumb>
					</Breadcrumbs>
					<FeatureStageBadge contentType="beta" size="sm" />
				</Stack>
				{permissions.createGroup && feats.template_rbac && (
					<Button component={RouterLink} startIcon={<GroupAdd />} to="create">
						Create group
					</Button>
				)}
			</Stack>

			<GroupsPageView
				organization={organization}
				groups={groupsQuery.data}
				canCreateGroup={permissions.createGroup}
				isTemplateRBACEnabled={feats.template_rbac}
			/>
		</>
	);
};

export default GroupsPage;

export const getOrganizationNameByDefault = (
	organizations: readonly Organization[],
) => {
	return organizations.find((org) => org.is_default)?.name;
};
