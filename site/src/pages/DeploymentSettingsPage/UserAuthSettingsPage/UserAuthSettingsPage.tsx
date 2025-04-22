import { useDeploymentConfig } from "modules/management/DeploymentConfigProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { UserAuthSettingsPageView } from "./UserAuthSettingsPageView";

const UserAuthSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();

	return (
		<>
			<Helmet>
				<title>{pageTitle("User Authentication Settings")}</title>
			</Helmet>
			<UserAuthSettingsPageView options={deploymentConfig.options} />
		</>
	);
};

export default UserAuthSettingsPage;
