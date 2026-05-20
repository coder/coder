import { useSearchParams } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import NotFoundPage from "#/pages/404Page/404Page";
import { pageTitle } from "#/utils/page";
import {
	findAddableProvider,
	isAddableProviderType,
} from "../components/addableProviderTypes";
import AddProviderPageView from "./AddProviderPageView";

const AddProviderPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const hasPermission = permissions.viewAnyAIProvider;
	const [searchParams] = useSearchParams();
	const typeParam = searchParams.get("type");

	// Unknown `?type` has no form schema to render against.
	if (!isAddableProviderType(typeParam)) {
		return <NotFoundPage />;
	}

	const provider = findAddableProvider(typeParam);
	const title = provider ? `New ${provider.label} Provider` : "New AI Provider";

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle(title, "AI Providers")}</title>

			<AddProviderPageView type={typeParam} />
		</RequirePermission>
	);
};

export default AddProviderPage;
