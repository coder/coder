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
	const permissionsQuery = useQuery(
		organizationsPermissions(organizations?.map((o) => o.id)),
	);
	const provisionersQuery = useQuery(provisionerDaemonGroups(organizationName));

	if (!entitlements.features.multiple_organizations.enabled) {
		return (
			<Paywall
				message="Provisioners"
				description="Provisioners run your Terraform to create templates and workspaces. You need a Premium license to use this feature for multiple organizations."
				documentationLink={docs("/")}
			/>
		);
	}

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	if (permissionsQuery.isLoading || provisionersQuery.isLoading) {
		return <Loader />;
	}

	const permissions = permissionsQuery.data;
	const provisioners = provisionersQuery.data;
	const error = permissionsQuery.error || provisionersQuery.error;
	if (error || !permissions || !provisioners) {
		return <ErrorAlert error={error} />;
	}

	return (
		<OrganizationProvisionersPageView
			buildInfo={buildInfoQuery.data}
			provisioners={provisioners}
		/>
	);
};

export default OrganizationProvisionersPage;

const getOrganizationByName = (
	organizations: readonly Organization[],
	name: string,
) => organizations.find((org) => org.name === name);
