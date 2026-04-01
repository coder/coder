import { buildInfo } from "api/queries/buildInfo";
import { provisionerDaemons } from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams, useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const OrganizationProvisionersPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const [searchParams, setSearchParams] = useSearchParams();
	const queryParams = {
		ids: searchParams.get("ids") ?? "",
		tags: searchParams.get("tags") ?? "",
		offline: searchParams.get("offline") === "true",
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const { entitlements } = useDashboard();
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	const provisionersQuery = useQuery({
		...provisionerDaemons(organizationName, {
			...queryParams,
			limit: 100,
		}),
	});

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const title = (
		<title>
			{pageTitle(
				"Provisioners",
				organization.display_name || organization.name,
			)}
		</title>
	);

	if (!organizationPermissions?.viewProvisioners) {
		return (
			<>
				{title}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	return (
		<>
			{title}
			<OrganizationProvisionersPageView
				showPaywall={!entitlements.features.multiple_organizations.enabled}
				error={provisionersQuery.error}
				provisioners={provisionersQuery.data}
				buildVersion={buildInfoQuery.data?.version}
				onRetry={provisionersQuery.refetch}
				filter={queryParams}
				onFilterChange={({ ids, offline }) => {
					setSearchParams({
						ids,
						offline: offline.toString(),
					});
				}}
			/>
		</>
	);
};

export default OrganizationProvisionersPage;
