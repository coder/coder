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
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import { IdpSyncHelpTooltip } from "./IdpSyncHelpTooltip";
import IdpSyncPageView from "./IdpSyncPageView";

const mockOIDCConfig = {
	allow_signups: true,
	client_id: "test",
	client_secret: "test",
	client_key_file: "test",
	client_cert_file: "test",
	email_domain: [],
	issuer_url: "test",
	scopes: [],
	ignore_email_verified: true,
	username_field: "",
	name_field: "",
	email_field: "",
	auth_url_params: {},
	ignore_user_info: true,
	organization_field: "",
	organization_mapping: {},
	organization_assign_default: true,
	group_auto_create: false,
	group_regex_filter: "^Coder-.*$",
	group_allow_list: [],
	groups_field: "groups",
	group_mapping: { group1: "developers", group2: "admin", group3: "auditors" },
	user_role_field: "roles",
	user_role_mapping: { role1: ["role1", "role2"] },
	user_roles_default: [],
	sign_in_text: "",
	icon_url: "",
	signups_disabled_text: "string",
	skip_issuer_checks: true,
};

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
					tooltip={<IdpSyncHelpTooltip />}
				/>
				<Stack direction="row" spacing={2}>
					<Button
						startIcon={<LaunchOutlined />}
						component="a"
						href={docs("/admin/auth#group-sync-enterprise")}
						target="_blank"
					>
						Setup IdP Sync
					</Button>
					<Button component={RouterLink} startIcon={<AddIcon />} to="export">
						Export Policy
					</Button>
				</Stack>
			</Stack>

			<IdpSyncPageView oidcConfig={mockOIDCConfig} />
		</>
	);
};

export default IdpSyncPage;
