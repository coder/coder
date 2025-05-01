import { getErrorMessage } from "api/errors";
import { groupsByOrganization } from "api/queries/groups";
import {
	groupIdpSyncSettings,
	organizationIdpSyncClaimFieldValues,
	patchGroupSyncSettings,
	patchRoleSyncSettings,
	roleIdpSyncSettings,
} from "api/queries/organizations";
import { organizationRoles } from "api/queries/roles";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError } from "components/GlobalSnackbar/utils";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Link } from "components/Link/Link";
import { Paywall } from "components/Paywall/Paywall";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueries, useQuery, useQueryClient } from "react-query";
import { useParams, useSearchParams } from "react-router-dom";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import IdpSyncPageView from "./IdpSyncPageView";

const IdpSyncPage: FC = () => {
	const queryClient = useQueryClient();
	// IdP sync does not have its own entitlement and is based on templace_rbac
	const { template_rbac: isIdpSyncEnabled } = useFeatureVisibility();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const [groupField, setGroupField] = useState("");
	const [roleField, setRoleField] = useState("");

	const [
		groupIdpSyncSettingsQuery,
		roleIdpSyncSettingsQuery,
		groupsQuery,
		rolesQuery,
	] = useQueries({
		queries: [
			groupIdpSyncSettings(organizationName),
			roleIdpSyncSettings(organizationName),
			groupsByOrganization(organizationName),
			organizationRoles(organizationName),
		],
	});

	useEffect(() => {
		if (!groupIdpSyncSettingsQuery.data) {
			return;
		}

		setGroupField(groupIdpSyncSettingsQuery.data.field);
	}, [groupIdpSyncSettingsQuery.data]);

	useEffect(() => {
		if (!roleIdpSyncSettingsQuery.data) {
			return;
		}

		setRoleField(roleIdpSyncSettingsQuery.data.field);
	}, [roleIdpSyncSettingsQuery.data]);

	const [searchParams] = useSearchParams();
	const tab = searchParams.get("tab") || "groups";
	const field = tab === "groups" ? groupField : roleField;

	const fieldValuesQuery = useQuery(
		field
			? organizationIdpSyncClaimFieldValues(organizationName, field)
			: { enabled: false },
	);

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const helmet = (
		<Helmet>
			<title>
				{pageTitle("IdP Sync", organization.display_name || organization.name)}
			</title>
		</Helmet>
	);

	if (!organizationPermissions?.viewIdpSyncSettings) {
		return (
			<>
				{helmet}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	const patchGroupSyncSettingsMutation = useMutation(
		patchGroupSyncSettings(organizationName, queryClient),
	);
	const patchRoleSyncSettingsMutation = useMutation(
		patchRoleSyncSettings(organizationName, queryClient),
	);

	const error =
		patchGroupSyncSettingsMutation.error ||
		patchRoleSyncSettingsMutation.error ||
		groupIdpSyncSettingsQuery.error ||
		roleIdpSyncSettingsQuery.error ||
		groupsQuery.error;

	const groupsMap = new Map<string, string>();
	if (groupsQuery.data) {
		for (const group of groupsQuery.data) {
			groupsMap.set(group.id, group.display_name || group.name);
		}
	}

	return (
		<>
			{helmet}

			<div className="flex flex-col gap-12">
				<header className="flex flex-row items-baseline justify-between">
					<div className="flex flex-col gap-2">
						<h1 className="text-3xl m-0">IdP Sync</h1>
						<p className="flex flex-row gap-1 text-sm text-content-secondary font-medium m-0">
							Automatically assign groups or roles to a user based on their IdP
							claims.
							<Link href={docs("/admin/users/idp-sync")}>View docs</Link>
						</p>
					</div>
				</header>
				<ChooseOne>
					<Cond condition={!isIdpSyncEnabled}>
						<Paywall
							message="IdP Sync"
							description="Configure group and role mappings to manage permissions outside of Coder. You need an Premium license to use this feature."
							documentationLink={docs("/admin/users/idp-sync")}
						/>
					</Cond>
					<Cond>
						<IdpSyncPageView
							tab={tab}
							groupSyncSettings={groupIdpSyncSettingsQuery.data}
							roleSyncSettings={roleIdpSyncSettingsQuery.data}
							claimFieldValues={fieldValuesQuery.data}
							groups={groupsQuery.data}
							groupsMap={groupsMap}
							roles={rolesQuery.data}
							organization={organization}
							onGroupSyncFieldChange={setGroupField}
							onRoleSyncFieldChange={setRoleField}
							error={error}
							onSubmitGroupSyncSettings={async (data) => {
								try {
									await patchGroupSyncSettingsMutation.mutateAsync(data);
									displaySuccess("IdP Group sync settings updated.");
								} catch (error) {
									displayError(
										getErrorMessage(
											error,
											"Failed to update IdP group sync settings",
										),
									);
								}
							}}
							onSubmitRoleSyncSettings={async (data) => {
								try {
									await patchRoleSyncSettingsMutation.mutateAsync(data);
									displaySuccess("IdP Role sync settings updated.");
								} catch (error) {
									displayError(
										getErrorMessage(
											error,
											"Failed to update IdP role sync settings",
										),
									);
								}
							}}
						/>
					</Cond>
				</ChooseOne>
			</div>
		</>
	);
};

export default IdpSyncPage;
