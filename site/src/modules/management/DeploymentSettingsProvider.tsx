import type { DeploymentConfig } from "api/api";
import { deploymentConfig } from "api/queries/deployment";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
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
	const { permissions } = useAuthenticated();
	const deploymentConfigQuery = useQuery(deploymentConfig());

	// The deployment settings page also contains users, audit logs, and groups
	// so this page must be visible if you can see any of these.
	const canViewDeploymentSettingsPage =
		permissions.viewDeploymentValues ||
		permissions.viewAllUsers ||
		permissions.viewAnyAuditLog;

	// Not a huge problem to unload the content in the event of an error,
	// because the sidebar rendering isn't tied to this. Even if the user hits
	// a 403 error, they'll still have navigation options
	if (deploymentConfigQuery.error) {
		return <ErrorAlert error={deploymentConfigQuery.error} />;
	}

	if (!deploymentConfigQuery.data) {
		return <Loader />;
	}

	return (
		<RequirePermission isFeatureVisible={canViewDeploymentSettingsPage}>
			<DeploymentSettingsContext.Provider
				value={{ deploymentConfig: deploymentConfigQuery.data }}
			>
				<Outlet />
			</DeploymentSettingsContext.Provider>
		</RequirePermission>
	);
};

export default DeploymentSettingsProvider;
