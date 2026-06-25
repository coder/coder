import { ArrowLeftIcon } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { addableProviders } from "../components/addableProviderTypes";
import AddProviderPageView from "./AddProviderPageView";

const AddProviderPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const hasPermission = permissions.viewAnyAIProvider;
	const [searchParams] = useSearchParams();
	const typeParam = searchParams.get("type");

	const provider = addableProviders.find((p) => p.value === typeParam);
	if (!provider) {
		return (
			<div className="flex flex-col items-start gap-4 pt-4 px-6">
				<Link to="/ai/settings/providers">
					<Button variant="subtle">
						<ArrowLeftIcon />
						<span>Back to providers</span>
					</Button>
				</Link>
				<Alert severity="warning">
					<AlertTitle>Provider type not found</AlertTitle>
					<AlertDescription>
						The provider type you are trying to add is not valid. Please try
						again.
					</AlertDescription>
				</Alert>
			</div>
		);
	}

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>
				{pageTitle(`New ${provider.label} Provider`, "AI Providers")}
			</title>

			<AddProviderPageView provider={provider} />
		</RequirePermission>
	);
};

export default AddProviderPage;
