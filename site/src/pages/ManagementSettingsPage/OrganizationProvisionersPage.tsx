import { buildInfo } from "api/queries/buildInfo";
import {
	organizationsPermissions,
	provisionerDaemonGroups,
} from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import { Paywall } from "components/Paywall/Paywall";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { docs } from "utils/docs";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const OrganizationProvisionersPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organizations } = useOrganizationSettings();
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
