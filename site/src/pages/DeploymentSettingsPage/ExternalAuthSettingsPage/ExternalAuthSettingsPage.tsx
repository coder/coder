import { Loader } from "components/Loader/Loader";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
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
