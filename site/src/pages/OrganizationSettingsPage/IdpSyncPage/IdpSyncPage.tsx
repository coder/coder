import { type FC, useState } from "react";
import { useMutation, useQueries, useQueryClient } from "react-query";
import { useParams, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { groupsByOrganization } from "#/api/queries/groups";
import {
	groupIdpSyncSettings,
	organizationIdpSyncClaimFieldValues,
	patchGroupSyncSettings,
	patchRoleSyncSettings,
	roleIdpSyncSettings,
} from "#/api/queries/organizations";
import { organizationRoles } from "#/api/queries/roles";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { PremiumPaywall } from "#/modules/paywall/PremiumPaywall";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";
import IdpSyncPageView from "./IdpSyncPageView";

const IdpSyncPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	// IdP sync does not have its own entitlement and is based on templace_rbac
	const { template_rbac: isIdpSyncEnabled } = useFeatureVisibility();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const [groupFieldOverride, setGroupFieldOverride] = useState<string>();
	const [roleFieldOverride, setRoleFieldOverride] = useState<string>();

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

	const [searchParams] = useSearchParams();
	const tab = searchParams.get("tab") === "roles" ? "roles" : "groups";
	const groupField =
		groupFieldOverride ?? groupIdpSyncSettingsQuery.data?.field ?? "";
	const roleField =
		roleFieldOverride ?? roleIdpSyncSettingsQuery.data?.field ?? "";

	const [groupFieldValuesQuery, roleFieldValuesQuery] = useQueries({
		queries: [
			{
				...organizationIdpSyncClaimFieldValues(organizationName, groupField),
				enabled: Boolean(groupField),
			},
			{
				...organizationIdpSyncClaimFieldValues(organizationName, roleField),
				enabled: Boolean(roleField),
			},
		],
	});

	const patchGroupSyncSettingsMutation = useMutation(
		patchGroupSyncSettings(organizationName, queryClient),
	);
	const patchRoleSyncSettingsMutation = useMutation(
		patchRoleSyncSettings(organizationName, queryClient),
	);

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const title = (
		<title>
			{pageTitle("IdP Sync", organization.display_name || organization.name)}
		</title>
	);

	if (!organizationPermissions?.viewIdpSyncSettings) {
		return (
			<>
				{title}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

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
		<div className="w-full max-w-(--breakpoint-2xl) pb-10">
			{title}

			<SettingsHeader>
				<SettingsHeaderTitle>IdP Sync</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Automatically assign groups or roles to a user based on their IdP
					claims.{" "}
					<SettingsHeaderDocsLink href={docs("/admin/users/idp-sync")} />
				</SettingsHeaderDescription>
			</SettingsHeader>
			{!isIdpSyncEnabled ? (
				<PremiumPaywall
					source="idp_sync"
					message="IdP Sync"
					description="Auto-sync groups & roles from your IdP."
					features={[
						"Sync groups & roles automatically",
						"Configured per organization",
						"No manual user assignment",
						"Works with your OIDC provider",
					]}
					canViewPremium={permissions.viewAllLicenses}
				/>
			) : (
				<IdpSyncPageView
					tab={tab}
					groupSyncSettings={groupIdpSyncSettingsQuery.data}
					roleSyncSettings={roleIdpSyncSettingsQuery.data}
					groupClaimFieldValues={groupFieldValuesQuery.data}
					roleClaimFieldValues={roleFieldValuesQuery.data}
					groups={groupsQuery.data}
					groupsMap={groupsMap}
					roles={rolesQuery.data}
					organization={organization}
					onGroupSyncFieldChange={setGroupFieldOverride}
					onRoleSyncFieldChange={setRoleFieldOverride}
					error={error}
					onSubmitGroupSyncSettings={async (data) => {
						const mutation = patchGroupSyncSettingsMutation.mutateAsync(data);
						await toast
							.promise(mutation, {
								loading: "Updating IdP group sync settings...",
								success: "IdP group sync settings updated.",
								error: (error) => ({
									message: getErrorMessage(
										error,
										"Failed to update IdP group sync settings.",
									),
									description: getErrorDetail(error),
								}),
							})
							.unwrap();
					}}
					onSubmitRoleSyncSettings={async (data) => {
						const mutation = patchRoleSyncSettingsMutation.mutateAsync(data);
						await toast
							.promise(mutation, {
								loading: "Updating IdP role sync settings...",
								success: "IdP role sync settings updated.",
								error: (error) => ({
									message: getErrorMessage(
										error,
										"Failed to update IdP role sync settings.",
									),
									description: getErrorDetail(error),
								}),
							})
							.unwrap();
					}}
				/>
			)}
		</div>
	);
};

export default IdpSyncPage;
