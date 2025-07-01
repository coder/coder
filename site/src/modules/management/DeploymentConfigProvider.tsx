import type { DeploymentConfig } from "api/api";
import { deploymentConfig } from "api/queries/deployment";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { type FC, createContext, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet } from "react-router-dom";

export const DeploymentConfigContext = createContext<
	DeploymentConfigValue | undefined
>(undefined);

type DeploymentConfigValue = Readonly<{
	deploymentConfig: DeploymentConfig;
}>;

export const useDeploymentConfig = (): DeploymentConfigValue => {
	const context = useContext(DeploymentConfigContext);
	if (!context) {
		throw new Error(
			`${useDeploymentConfig.name} should be used inside of ${DeploymentConfigProvider.name}`,
		);
	}

	return context;
};

const DeploymentConfigProvider: FC = () => {
	const deploymentConfigQuery = useQuery(deploymentConfig());

	if (deploymentConfigQuery.error) {
		return <ErrorAlert error={deploymentConfigQuery.error} />;
	}

	if (!deploymentConfigQuery.data) {
		return <Loader />;
	}

	return (
		<DeploymentConfigContext.Provider
			value={{ deploymentConfig: deploymentConfigQuery.data }}
		>
			<Outlet />
		</DeploymentConfigContext.Provider>
	);
};

export default DeploymentConfigProvider;
