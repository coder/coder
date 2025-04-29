import { getErrorMessage } from "api/errors";
import { deploymentIdpSyncFieldValues } from "api/queries/deployment";
import {
	organizationIdpSyncSettings,
	patchOrganizationSyncSettings,
} from "api/queries/idpsync";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { displayError } from "components/GlobalSnackbar/utils";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import { Paywall } from "components/Paywall/Paywall";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { ExportPolicyButton } from "./ExportPolicyButton";
import { IdpOrgSyncPageView } from "./IdpOrgSyncPageView";

const IdpOrgSyncPage: FC = () => {
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

	const fieldValuesQuery = useQuery(
		field ? deploymentIdpSyncFieldValues(field) : { enabled: false },
	);

	const patchOrganizationSyncSettingsMutation = useMutation(
		patchOrganizationSyncSettings(queryClient),
	);

	useEffect(() => {
		if (patchOrganizationSyncSettingsMutation.error) {
			displayError(
				getErrorMessage(
					patchOrganizationSyncSettingsMutation.error,
					"Error updating organization idp sync settings.",
				),
			);
		}
	}, [patchOrganizationSyncSettingsMutation.error]);

	if (settingsQuery.isLoading) {
		return <Loader />;
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle("Organization IdP Sync")}</title>
			</Helmet>

			<div className="flex flex-col gap-12">
				<header className="flex flex-row items-baseline justify-between">
					<div className="flex flex-col gap-2">
						<h1 className="text-3xl m-0">Organization IdP Sync</h1>
						<p className="flex flex-row gap-1 text-sm text-content-secondary font-medium m-0">
							Automatically assign users to an organization based on their IdP
							claims.
							<Link href={docs("/admin/users/idp-sync#organization-sync")}>
								View docs
							</Link>
						</p>
					</div>
					<ExportPolicyButton syncSettings={settingsQuery.data} />
				</header>
				<ChooseOne>
					<Cond condition={!isIdpSyncEnabled}>
						<Paywall
							message="IdP Organization Sync"
							description="Configure organization mappings to synchronize claims in your auth provider to organizations within Coder. You need an Premium license to use this feature."
							documentationLink={docs("/admin/users/idp-sync")}
						/>
					</Cond>
					<Cond>
						<IdpOrgSyncPageView
							organizationSyncSettings={settingsQuery.data}
							claimFieldValues={fieldValuesQuery.data}
							organizations={organizations}
							onSyncFieldChange={setField}
							onSubmit={async (data) => {
								try {
									await patchOrganizationSyncSettingsMutation.mutateAsync(data);
									displaySuccess("Organization sync settings updated.");
								} catch (error) {
									displayError(
										getErrorMessage(
											error,
											"Failed to update organization IdP sync settings",
										),
									);
								}
							}}
							error={settingsQuery.error || fieldValuesQuery.error}
						/>
					</Cond>
				</ChooseOne>
			</div>
		</>
	);
};

export default IdpOrgSyncPage;
