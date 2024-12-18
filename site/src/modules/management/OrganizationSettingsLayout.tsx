import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, Suspense, createContext, useContext } from "react";
import { Outlet, useParams } from "react-router-dom";
import { OrganizationSidebar } from "./OrganizationSidebar";

export const OrganizationSettingsContext = createContext<
	OrganizationSettingsValue | undefined
>(undefined);

type OrganizationSettingsValue = Readonly<{
	organizations: readonly Organization[];
	organization?: Organization;
}>;

export const useOrganizationSettings = (): OrganizationSettingsValue => {
	const context = useContext(OrganizationSettingsContext);
	if (!context) {
		throw new Error(
			"useOrganizationSettings should be used inside of OrganizationSettingsLayout",
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

const OrganizationSettingsLayout: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const { organization: orgName } = useParams() as {
		organization?: string;
	};

	const canViewOrganizationSettingsPage =
		permissions.viewDeploymentValues || permissions.editAnyOrganization;

	const organization =
		organizations && orgName
			? organizations.find((org) => org.name === orgName)
			: undefined;

	return (
		<RequirePermission isFeatureVisible={canViewOrganizationSettingsPage}>
			<OrganizationSettingsContext.Provider
				value={{
					organizations,
					organization,
				}}
			>
				<div className="px-10 max-w-screen-2xl">
					<div className="flex flex-row gap-12 py-10">
						<OrganizationSidebar />
						<main css={{ flexGrow: 1 }}>
							<Suspense fallback={<Loader />}>
								<Outlet />
							</Suspense>
						</main>
					</div>
				</div>
			</OrganizationSettingsContext.Provider>
		</RequirePermission>
	);
};

export default OrganizationSettingsLayout;
