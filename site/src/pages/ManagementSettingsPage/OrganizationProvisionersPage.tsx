import { buildInfo } from "api/queries/buildInfo";
import { provisionerDaemonGroups } from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const OrganizationProvisionersPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organizations } = useManagementSettings();
	const { entitlements } = useDashboard();

	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));

	const organization = organizations
		? getOrganizationByName(organizations, organizationName)
		: undefined;
	const provisionersQuery = useQuery(provisionerDaemonGroups(organizationName));

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	return (
		<OrganizationProvisionersPageView
			showPaywall={!entitlements.features.multiple_organizations.enabled}
			error={provisionersQuery.error}
			buildInfo={buildInfoQuery.data}
			provisioners={provisionersQuery.data}
		/>
	);
};

export default OrganizationProvisionersPage;

const getOrganizationByName = (
	organizations: readonly Organization[],
	name: string,
) => organizations.find((org) => org.name === name);
