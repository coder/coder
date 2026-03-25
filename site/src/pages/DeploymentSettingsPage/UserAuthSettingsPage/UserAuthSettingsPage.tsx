import type { FC } from "react";
import { pageTitle } from "utils/page";
import { useDeploymentConfig } from "#/modules/management/DeploymentConfigProvider";
import { UserAuthSettingsPageView } from "./UserAuthSettingsPageView";

const UserAuthSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentConfig();

	return (
		<>
			<title>{pageTitle("User Authentication Settings")}</title>

			<UserAuthSettingsPageView options={deploymentConfig.options} />
		</>
	);
};

export default UserAuthSettingsPage;
