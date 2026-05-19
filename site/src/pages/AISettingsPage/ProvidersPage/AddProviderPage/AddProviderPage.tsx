import { useQuery } from "react-query";
import { organizations } from "#/api/queries/organizations";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import AddProviderPageView from "./AddProviderPageView";

const AddProviderPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const { metadata } = useEmbeddedMetadata();
	const hasPermission = permissions.viewAnyAIProvider;

	const organizationsQuery = useQuery(organizations(metadata.organizations));

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("New AI Provider", "AI Providers")}</title>

			<AddProviderPageView organizations={organizationsQuery.data} />
		</RequirePermission>
	);
};

export default AddProviderPage;
