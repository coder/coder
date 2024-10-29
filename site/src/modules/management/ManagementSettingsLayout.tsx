import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, Suspense, createContext, useContext } from "react";
import { Outlet, useParams } from "react-router-dom";
import { Sidebar } from "./Sidebar";

export const ManagementSettingsContext = createContext<
	ManagementSettingsValue | undefined
>(undefined);

type ManagementSettingsValue = Readonly<{
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

const ManagementSettingsLayout: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const { organization: orgName } = useParams() as {
		organization?: string;
	};

	// The deployment settings page also contains users, audit logs, groups and
	// organizations, so this page must be visible if you can see any of these.
	const canViewDeploymentSettingsPage =
		permissions.viewDeploymentValues ||
		permissions.viewAllUsers ||
		permissions.editAnyOrganization ||
		permissions.viewAnyAuditLog;

	const organization =
		organizations && orgName
			? organizations.find((org) => org.name === orgName)
			: undefined;

	return (
		<RequirePermission isFeatureVisible={canViewDeploymentSettingsPage}>
			<ManagementSettingsContext.Provider
				value={{
					organizations,
					organization,
				}}
			>
				<Margins>
					<Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
						<Sidebar />
						<main css={{ flexGrow: 1 }}>
							<Suspense fallback={<Loader />}>
								<Outlet />
							</Suspense>
						</main>
					</Stack>
				</Margins>
			</ManagementSettingsContext.Provider>
		</RequirePermission>
	);
};

export default ManagementSettingsLayout;
