import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import { getErrorMessage } from "api/errors";
import {
	organizationIdpSyncSettings,
	patchOrganizationSyncSettings,
} from "api/queries/idpsync";
import type { OrganizationSyncSettings } from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError } from "components/GlobalSnackbar/utils";
import { Paywall } from "components/Paywall/Paywall";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import IdpOrgSyncPageView from "./IdpOrgSyncPageView";

export const IdpOrgSyncPage: FC = () => {
	const queryClient = useQueryClient();
	// IdP sync does not have its own entitlement and is based on templace_rbac
	const { template_rbac: isIdpSyncEnabled } = useFeatureVisibility();
	const { organizations } = useDashboard();
	const organizationIdpSyncSettingsQuery = useQuery(
		organizationIdpSyncSettings(),
	);
	const patchOrganizationSyncSettingsMutation = useMutation(
		patchOrganizationSyncSettings(queryClient),
	);

	const error = organizationIdpSyncSettingsQuery.error;

	return (
		<>
			<Helmet>
				<title>{pageTitle("Organization IdP Sync")}</title>
			</Helmet>

			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader
					title="Organization IdP Sync"
					description="Automatically assign users to an organization based on their IDP claims."
				/>
				<Button
					startIcon={<LaunchOutlined />}
					component="a"
					href={docs("/admin/users/idp-sync")}
					target="_blank"
				>
					Setup IdP Sync
				</Button>
			</Stack>
			<ChooseOne>
				<Cond condition={!isIdpSyncEnabled}>
					<Paywall
						message="IdP Sync"
						description="Configure group and role mappings to manage permissions outside of Coder. You need an Premium license to use this feature."
						documentationLink={docs("/admin/users/idp-sync")}
					/>
				</Cond>
				<Cond>
					<IdpOrgSyncPageView
						organizationSyncSettings={organizationIdpSyncSettingsQuery.data}
						organizations={organizations}
						onSubmit={async (data: OrganizationSyncSettings) => {
							try {
								// await patchOrganizationSyncSettingsMutation.mutateAsync(data);
								console.log("submit form", data);
							} catch (error) {
								displayError(
									getErrorMessage(
										error,
										"Failed to organization IdP sync settings",
									),
								);
							}
						}}
						error={error || patchOrganizationSyncSettingsMutation.error}
						// isLoading={patchOrganizationSyncSettingsMutation.isLoading}
					/>
				</Cond>
			</ChooseOne>
		</>
	);
};

export default IdpOrgSyncPage;
