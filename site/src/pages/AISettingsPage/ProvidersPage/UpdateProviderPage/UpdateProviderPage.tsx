import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import UpdateProviderPageView from "./UpdateProviderPageView";

const UpdateProviderPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const hasPermission = permissions.viewAnyAIProvider;

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<UpdateProviderPageView />
		</RequirePermission>
	);
};

export default UpdateProviderPage;
