import { buildInfo } from "api/queries/buildInfo";
import { provisionerDaemonGroups } from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
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
	const { organization } = useOrganizationSettings();
	const { entitlements } = useDashboard();
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	const provisionersQuery = useQuery(provisionerDaemonGroups(organizationName));

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	return (
		<>
			<Helmet>
				<title>
					{pageTitle(
						"Provisioners",
						organization.display_name || organization.name,
					)}
				</title>
			</Helmet>
			<OrganizationProvisionersPageView
				showPaywall={!entitlements.features.multiple_organizations.enabled}
				error={provisionersQuery.error}
				buildInfo={buildInfoQuery.data}
				provisioners={provisionersQuery.data}
			/>
		</>
	);
};

export default OrganizationProvisionersPage;
