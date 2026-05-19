import { useQuery } from "react-query";
import { aiProvidersList } from "#/api/queries/aiProviders";
import { organizations } from "#/api/queries/organizations";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import ProvidersPageView from "#/pages/AISettingsPage/ProvidersPage/ProvidersPageView";
import { pageTitle } from "#/utils/page";

const ProvidersPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const { metadata } = useEmbeddedMetadata();
	const hasPermission = permissions.viewAnyAIProvider;

	const providersQuery = useQuery(aiProvidersList());
	const organizationsQuery = useQuery(organizations(metadata.organizations));

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("AI Providers")}</title>

			<ProvidersPageView
				isLoading={providersQuery.isLoading}
				isFetching={providersQuery.isFetching}
				providers={providersQuery.data ?? []}
				organizations={organizationsQuery.data}
			/>
		</RequirePermission>
	);
};

export default ProvidersPage;
