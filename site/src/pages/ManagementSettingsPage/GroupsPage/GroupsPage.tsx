import GroupAdd from "@mui/icons-material/GroupAddOutlined";
import Button from "@mui/material/Button";
import { getErrorMessage } from "api/errors";
import { groupsByOrganization } from "api/queries/groups";
import { organizationPermissions } from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Navigate, Link as RouterLink, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import GroupsPageView from "./GroupsPageView";

export const GroupsPage: FC = () => {
	const feats = useFeatureVisibility();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const groupsQuery = useQuery(groupsByOrganization(organizationName));
	const { organizations } = useManagementSettings();
	const organization = organizations?.find((o) => o.name === organizationName);
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

	if (!organizations) {
		return <Loader />;
	}

	if (!organizationName) {
		const defaultName = getOrganizationNameByDefault(organizations);
		if (defaultName) {
			return <Navigate to={`/organizations/${defaultName}/groups`} replace />;
		}
		// We expect there to always be a default organization.
		throw new Error("No default organization found");
	}

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
			>
				<SettingsHeader
					title="Groups"
					description="Manage groups for this organization."
				/>
				{permissions.createGroup && feats.template_rbac && (
					<Button component={RouterLink} startIcon={<GroupAdd />} to="create">
						Create group
					</Button>
				)}
			</Stack>

			<GroupsPageView
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
