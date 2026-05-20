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

	// The page is reachable only through the "Add provider" dropdown,
	// which always appends `?type=<known>`. Anyone hitting a stale
	// bookmark or an unknown type gets the 404 page; without a known
	// type the form has no schema to render against.
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
