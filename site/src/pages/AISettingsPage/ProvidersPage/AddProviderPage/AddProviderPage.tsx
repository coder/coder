import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import AddProviderPageView from "./AddProviderPageView";

const AddProviderPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const hasPermission = permissions.viewAnyAIProvider;

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("New AI Provider", "AI Providers")}</title>

			<AddProviderPageView />
		</RequirePermission>
	);
};

export default AddProviderPage;
