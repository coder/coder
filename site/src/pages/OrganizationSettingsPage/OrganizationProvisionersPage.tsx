import { buildInfo } from "api/queries/buildInfo";
import { provisionerDaemonGroups } from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const OrganizationProvisionersPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const { entitlements } = useDashboard();
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	
	// Only query for provisioners if we have a valid organization
	const provisionersQuery = useQuery({
		...provisionerDaemonGroups(organizationName),
		enabled: !!organization
	});

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const helmet = (
		<Helmet>
			<title>
				{pageTitle(
					"Provisioners",
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

	// In OSS mode, we should always show the provisioners page without a paywall
	const showPaywall = entitlements.has_license && !entitlements.features.multiple_organizations.enabled;

	return (
		<>
			{helmet}
			<OrganizationProvisionersPageView
				showPaywall={showPaywall}
				error={provisionersQuery.error}
				buildInfo={buildInfoQuery.data}
				provisioners={provisionersQuery.data}
			/>
		</>
	);
};

export default OrganizationProvisionersPage;
