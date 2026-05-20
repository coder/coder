import { Navigate, useSearchParams } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import {
	findAddableProvider,
	isSupportedAddableProviderType,
} from "../components/addableProviderTypes";
import AddProviderPageView from "./AddProviderPageView";

const AddProviderPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const hasPermission = permissions.viewAnyAIProvider;
	const [searchParams] = useSearchParams();
	const typeParam = searchParams.get("type");

	// The page is reachable only through the "Add provider" dropdown,
	// which always appends `?type=<known>`. Anyone hitting a stale
	// bookmark, an unknown type, or a known-but-not-yet-supported type
	// (Azure, Google, OpenAI via bridge, OpenRouter, Vercel) is sent
	// back to the list page; without a known type the form has no
	// schema to render against.
	if (!isSupportedAddableProviderType(typeParam)) {
		return <Navigate to="/ai/settings" replace />;
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
