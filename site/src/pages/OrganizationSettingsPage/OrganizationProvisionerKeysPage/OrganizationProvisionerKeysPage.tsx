import { provisionerDaemonGroups } from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { OrganizationProvisionerKeysPageView } from "./OrganizationProvisionerKeysPageView";

const OrganizationProvisionerKeysPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const { entitlements } = useDashboard();
	const provisionerKeyDaemonsQuery = useQuery({
		...provisionerDaemonGroups(organizationName),
		select: (data) =>
			[...data].sort((a, b) => b.daemons.length - a.daemons.length),
	});

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const helmet = (
		<Helmet>
			<title>
				{pageTitle(
					"Provisioner Keys",
					organization.display_name || organization.name,
				)}
			</title>
		</Helmet>
	);

	if (!organizationPermissions?.viewProvisioners) {
		return (
			<>
				{helmet}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	return (
		<>
			{helmet}
			<OrganizationProvisionerKeysPageView
				showPaywall={!entitlements.features.multiple_organizations.enabled}
				provisionerKeyDaemons={provisionerKeyDaemonsQuery.data}
				error={provisionerKeyDaemonsQuery.error}
				onRetry={provisionerKeyDaemonsQuery.refetch}
			/>
		</>
	);
};

export default OrganizationProvisionerKeysPage;
