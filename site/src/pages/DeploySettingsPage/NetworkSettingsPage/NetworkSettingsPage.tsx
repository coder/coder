import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { Loader } from "components/Loader/Loader";
import { pageTitle } from "utils/page";
import { useDeploySettings } from "../DeploySettingsLayout";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";

const NetworkSettingsPage: FC = () => {
	const { deploymentValues } = useDeploySettings();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Network Settings")}</title>
			</Helmet>

			{deploymentValues ? (
				<NetworkSettingsPageView options={deploymentValues.options} />
			) : (
				<Loader />
			)}
		</>
	);
};

export default NetworkSettingsPage;
