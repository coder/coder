import { deploymentConfig } from "api/queries/deployment";
import type {
	AuthorizationResponse,
	DeploymentConfig,
	Organization,
} from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import { createContext, type FC, Suspense, useContext } from "react";
import { useQuery } from "react-query";
import { Navigate, Outlet, useNavigate, useParams } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export const ManagementSettingsContext = createContext<
	{ deploymentValues: DeploymentConfig } | undefined
>(undefined);

type ManagementSettingsValue = Readonly<{
	deploymentValues: DeploymentConfig;
	organizations: readonly Organization[];
	organization?: Organization;
}>;

export const useManagementSettings = (): ManagementSettingsValue => {
	const context = useContext(ManagementSettingsContext);
	if (!context) {
		throw new Error(
			"useManagementSettings should be used inside of ManagementSettingsLayout",
		);
	}
	const { organizations } = useDashboard();
	const { organization: orgName } = useParams() as {
		organization?: string;
	};

	const organization =
		organizations && orgName
			? organizations.find((org) => org.name === orgName)
			: undefined;

	return {
		deploymentValues: context.deploymentValues,
		organizations,
		organization,
	};
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
	const deploymentConfigQuery = useQuery(deploymentConfig());

	// The deployment settings page also contains users, audit logs, groups and
	// organizations, so this page must be visible if you can see any of these.
	const canViewDeploymentSettingsPage =
		permissions.viewDeploymentValues ||
		permissions.viewAllUsers ||
		permissions.editAnyOrganization ||
		permissions.viewAnyAuditLog;

	if (deploymentConfigQuery.error) {
		return <ErrorAlert error={deploymentConfigQuery.error} />;
	}

	if (!deploymentConfigQuery.data) {
		return <Loader />;
	}

	return (
		<Margins>
			<Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
				<Sidebar />
				<main css={{ width: "100%" }}>
					<Suspense fallback={<Loader />}>
						<RequirePermission isFeatureVisible={canViewDeploymentSettingsPage}>
							<ManagementSettingsContext.Provider
								value={{ deploymentValues: deploymentConfigQuery.data }}
							>
								<Outlet />
							</ManagementSettingsContext.Provider>
						</RequirePermission>
					</Suspense>
				</main>
			</Stack>
		</Margins>
	);
};
