import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { deploymentIdpSyncFieldValues } from "#/api/queries/deployment";
import {
	organizationIdpSyncSettings,
	patchOrganizationSyncSettings,
} from "#/api/queries/idpsync";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { ExportPolicyButton } from "#/modules/idpSync/ExportPolicyButton";
import { PremiumPaywall } from "#/modules/paywall/PremiumPaywall";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";
import { IdpOrgSyncPageView } from "./IdpOrgSyncPageView";

const IdpOrgSyncPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	// IdP sync does not have its own entitlement and is based on templace_rbac
	const { template_rbac: isIdpSyncEnabled } = useFeatureVisibility();
	const { organizations } = useDashboard();
	const settingsQuery = useQuery(organizationIdpSyncSettings(isIdpSyncEnabled));
	const [fieldOverride, setFieldOverride] = useState<string>();
	const field = fieldOverride ?? settingsQuery.data?.field ?? "";

	const fieldValuesQuery = useQuery({
		...deploymentIdpSyncFieldValues(field),
		enabled: Boolean(field),
	});

	const patchOrganizationSyncSettingsMutation = useMutation(
		patchOrganizationSyncSettings(queryClient),
	);

	if (settingsQuery.isLoading) {
		return <Loader />;
	}

	return (
		<>
			<title>{pageTitle("Organization IdP Sync")}</title>

			<div>
				<SettingsHeader
					actions={
						<ExportPolicyButton
							syncSettings={settingsQuery.data}
							filename="organizations_policy.json"
						/>
					}
				>
					<SettingsHeaderTitle>Organization IdP Sync</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Automatically assign users to an organization based on their IdP
						claims.{" "}
						<SettingsHeaderDocsLink
							href={docs("/admin/users/idp-sync#organization-sync")}
						/>
					</SettingsHeaderDescription>
				</SettingsHeader>
				{!isIdpSyncEnabled ? (
					<PremiumPaywall
						source="idp_org_sync"
						message="IdP Organization Sync"
						description="Configure organization mappings to synchronize claims in your auth provider to organizations within Coder."
						features={[
							"Sync groups & roles automatically",
							"No manual user assignment",
							"Works with your OIDC provider",
						]}
						canViewPremium={permissions.viewAllLicenses}
					/>
				) : (
					<IdpOrgSyncPageView
						organizationSyncSettings={settingsQuery.data}
						claimFieldValues={fieldValuesQuery.data}
						organizations={organizations}
						onSyncFieldChange={setFieldOverride}
						onSubmit={async (data) => {
							const mutation =
								patchOrganizationSyncSettingsMutation.mutateAsync(data);
							toast.promise(mutation, {
								loading: "Updating organization IdP sync settings...",
								success: "Organization IdP sync settings updated.",
								error: (error) => ({
									message: getErrorMessage(
										error,
										"Failed to update organization IdP sync settings.",
									),
									description: getErrorDetail(error),
								}),
							});
						}}
						error={settingsQuery.error || fieldValuesQuery.error}
					/>
				)}
			</div>
		</>
	);
};

export default IdpOrgSyncPage;
