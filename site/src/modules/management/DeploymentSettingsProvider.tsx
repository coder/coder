import type { DeploymentConfig } from "api/api";
import { deploymentConfig } from "api/queries/deployment";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { type FC, createContext, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet } from "react-router-dom";

export const DeploymentSettingsContext = createContext<
	DeploymentSettingsValue | undefined
>(undefined);

type DeploymentSettingsValue = Readonly<{
	deploymentConfig: DeploymentConfig;
}>;

export const useDeploymentSettings = (): DeploymentSettingsValue => {
	const context = useContext(DeploymentSettingsContext);
	if (!context) {
		throw new Error(
			`${useDeploymentSettings.name} should be used inside of ${DeploymentSettingsProvider.name}`,
		);
	}

	return context;
};

const DeploymentSettingsProvider: FC = () => {
	const deploymentConfigQuery = useQuery(deploymentConfig());

	if (deploymentConfigQuery.error) {
		return <ErrorAlert error={deploymentConfigQuery.error} />;
	}

	if (!deploymentConfigQuery.data) {
		return <Loader />;
	}

	return (
		<DeploymentSettingsContext.Provider
			value={{ deploymentConfig: deploymentConfigQuery.data }}
		>
			<Outlet />
		</DeploymentSettingsContext.Provider>
	);
};

export default DeploymentSettingsProvider;
