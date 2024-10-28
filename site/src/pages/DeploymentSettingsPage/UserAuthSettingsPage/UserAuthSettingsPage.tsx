import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { UserAuthSettingsPageView } from "./UserAuthSettingsPageView";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";

const UserAuthSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();

	return (
		<>
			<Helmet>
				<title>{pageTitle("User Authentication Settings")}</title>
			</Helmet>

			{deploymentConfig ? (
				<UserAuthSettingsPageView options={deploymentConfig.options} />
			) : (
				<Loader />
			)}
		</>
	);
};

export default UserAuthSettingsPage;
