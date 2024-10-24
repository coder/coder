import type { DeploymentConfig } from "api/api";
import { deploymentConfig } from "api/queries/deployment";
import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, Suspense, createContext, useContext } from "react";
import { useQuery } from "react-query";
import { Navigate, Outlet, useParams } from "react-router-dom";
import { Sidebar } from "./Sidebar";

type ManagementSettingsValue = Readonly<{
	deploymentValues: DeploymentConfig;
}>;

export const ManagementSettingsContext = createContext<
	ManagementSettingsValue | undefined
>(undefined);

export const useManagementSettings = (): ManagementSettingsValue => {
	const context = useContext(ManagementSettingsContext);
	if (!context) {
		throw new Error(
			"useManagementSettings should be used inside of ManagementSettingsLayout",
		);
	}

	return context;
};

/**
 * Return true if the user can edit the organization settings or its members.
 */
export const canEditOrganization = (
	permissions: AuthorizationResponse | undefined,
) => {
	return (
		permissions !== undefined &&
		(permissions.editOrganization ||
			permissions.editMembers ||
			permissions.editGroups)
	);
};

/**
 * A multi-org capable settings page layout.
 *
 * If multi-org is not enabled or licensed, this is the wrong layout to use.
 * See DeploySettingsLayoutInner instead.
 */
export const ManagementSettingsLayout: FC = () => {
	const { permissions } = useAuthenticated();

	// The deployment settings page also contains users, audit logs, groups and
	// organizations, so this page must be visible if you can see any of these.
	const canViewDeploymentSettingsPage =
		permissions.viewDeploymentValues ||
		permissions.viewAllUsers ||
		permissions.editAnyOrganization ||
		permissions.viewAnyAuditLog;

	const deploymentConfigQuery = useQuery({
		...deploymentConfig(),
		enabled: canViewDeploymentSettingsPage,
	});

	// Need to make sure that we redirect basic users. If the enabled value for
	// deploymentConfigQuery is false, the query object will be stuck in a
	// loading state forever
	if (!canViewDeploymentSettingsPage) {
		return <Navigate to="/workspaces" replace />;
	}

	if (deploymentConfigQuery.isLoading) {
		return <Loader />;
	}

	return (
		<RequirePermission isFeatureVisible={canViewDeploymentSettingsPage}>
			<Margins>
				<Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
					<Sidebar />

					<main css={{ flexGrow: 1 }}>
						{deploymentConfigQuery.isError ? (
							<ErrorAlert error={deploymentConfigQuery.error} />
						) : (
							<ManagementSettingsContext.Provider
								value={{
									deploymentValues: deploymentConfigQuery.data,
								}}
							>
								<Suspense fallback={<Loader />}>
									<Outlet />
								</Suspense>
							</ManagementSettingsContext.Provider>
						)}
					</main>
				</Stack>
			</Margins>
		</RequirePermission>
	);
};
