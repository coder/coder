import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { SecuritySettingsPageView } from "./SecuritySettingsPageView";

const SecuritySettingsPage: FC = () => {
	const { deploymentValues } = useManagementSettings();
	const { entitlements } = useDashboard();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Security Settings")}</title>
			</Helmet>

			{deploymentValues ? (
				<SecuritySettingsPageView
					options={deploymentValues.options}
					featureBrowserOnlyEnabled={entitlements.features.browser_only.enabled}
				/>
			) : (
				<Loader />
			)}
		</>
	);
};

export default SecuritySettingsPage;
