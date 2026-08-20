import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { deploymentIdpSyncFieldValues } from "#/api/queries/deployment";
import {
	organizationIdpSyncSettings,
	patchOrganizationSyncSettings,
} from "#/api/queries/idpsync";
import { Link } from "#/components/Link/Link";
import { Loader } from "#/components/Loader/Loader";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";
import { ExportPolicyButton } from "./ExportPolicyButton";
import { IdpOrgSyncPageView } from "./IdpOrgSyncPageView";

const IdpOrgSyncPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	// IdP sync does not have its own entitlement and is based on templace_rbac
	const { template_rbac: isIdpSyncEnabled } = useFeatureVisibility();
	const { organizations } = useDashboard();
	const settingsQuery = useQuery(organizationIdpSyncSettings(isIdpSyncEnabled));

	const [field, setField] = useState("");
	useEffect(() => {
		if (!settingsQuery.data) {
			return;
		}

		setField(settingsQuery.data.field);
	}, [settingsQuery.data]);

	const fieldValuesQuery = useQuery({
		...deploymentIdpSyncFieldValues(field),
		enabled: Boolean(field),
	});

	const patchOrganizationSyncSettingsMutation = useMutation(
		patchOrganizationSyncSettings(queryClient),
	);

	useEffect(() => {
		if (patchOrganizationSyncSettingsMutation.error) {
			toast.error(
				getErrorMessage(
					patchOrganizationSyncSettingsMutation.error,
					"Error updating organization IdP sync settings.",
				),
			);
		}
	}, [patchOrganizationSyncSettingsMutation.error]);

	if (settingsQuery.isLoading) {
		return <Loader />;
	}

	return (
		<>
			<title>{pageTitle("Organization IdP Sync")}</title>

			<div>
				<SettingsHeader
					actions={<ExportPolicyButton syncSettings={settingsQuery.data} />}
				>
					<SettingsHeaderTitle>Organization IdP Sync</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Automatically assign users to an organization based on their IdP
						claims.{" "}
						<Link href={docs("/admin/users/idp-sync#organization-sync")}>
							View docs
						</Link>
					</SettingsHeaderDescription>
				</SettingsHeader>
				{!isIdpSyncEnabled ? (
					<PaywallPremium
						message="IdP Organization Sync"
						description="Configure organization mappings to synchronize claims in your auth provider to organizations within Coder."
						canViewPremium={permissions.viewAllLicenses}
					/>
				) : (
					<IdpOrgSyncPageView
						organizationSyncSettings={settingsQuery.data}
						claimFieldValues={fieldValuesQuery.data}
						organizations={organizations}
						onSyncFieldChange={setField}
						onSubmit={async (data) => {
							try {
								await patchOrganizationSyncSettingsMutation.mutateAsync(data);
								toast.success("Organization sync settings updated.");
							} catch (error) {
								toast.error(
									getErrorMessage(
										error,
										"Failed to update organization IdP sync settings.",
									),
									{
										description: getErrorDetail(error),
									},
								);
							}
						}}
						error={settingsQuery.error || fieldValuesQuery.error}
					/>
				)}
			</div>
		</>
	);
};

export default IdpOrgSyncPage;
