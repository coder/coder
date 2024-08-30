import AddIcon from "@mui/icons-material/AddOutlined";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import { getErrorMessage } from "api/errors";
import { organizationPermissions } from "api/queries/organizations";
import { organizationRoles } from "api/queries/roles";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery, useQueryClient } from "react-query";
import { Link as RouterLink, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import { docs } from "utils/docs";
import IdpSyncPageView from "./IdpSyncPageView";

export const IdpSyncPage: FC = () => {
	const queryClient = useQueryClient();
	// const { custom_roles: isCustomRolesEnabled } = useFeatureVisibility();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organizations } = useOrganizationSettings();
	const organization = organizations?.find((o) => o.name === organizationName);
	const permissionsQuery = useQuery(organizationPermissions(organization?.id));
	const organizationRolesQuery = useQuery(organizationRoles(organizationName));
	const permissions = permissionsQuery.data;

	useEffect(() => {
		if (organizationRolesQuery.error) {
			displayError(
				getErrorMessage(
					organizationRolesQuery.error,
					"Error loading custom roles.",
				),
			);
		}
	}, [organizationRolesQuery.error]);

	if (!permissions) {
		return <Loader />;
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle("IdP Sync")}</title>
			</Helmet>

			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader
					title="IdP Sync"
					description="Group and role sync mappings (configured outside Coder)."
				/>
				<Stack direction="row" spacing={2}>
					<Button
						startIcon={<LaunchOutlined />}
						component="a"
						href={docs("/cli/server#--notifications-webhook-endpoint")}
						target="_blank"
					>
						Setup IdP Sync
					</Button>
					<Button component={RouterLink} startIcon={<AddIcon />} to="export">
						Export Policy
					</Button>
				</Stack>
			</Stack>

			<IdpSyncPageView roles={[]} />
		</>
	);
};

export default IdpSyncPage;
