import { Loader } from "components/Loader/Loader";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { UserAuthSettingsPageView } from "./UserAuthSettingsPageView";

const UserAuthSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();

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
