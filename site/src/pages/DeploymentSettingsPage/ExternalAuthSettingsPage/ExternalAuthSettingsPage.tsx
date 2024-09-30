import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";

const ExternalAuthSettingsPage: FC = () => {
	const { deploymentValues } = useManagementSettings();

	return (
		<>
			<Helmet>
				<title>{pageTitle("External Authentication Settings")}</title>
			</Helmet>

			{deploymentValues ? (
				<ExternalAuthSettingsPageView config={deploymentValues.config} />
			) : (
				<Loader />
			)}
		</>
	);
};

export default ExternalAuthSettingsPage;
