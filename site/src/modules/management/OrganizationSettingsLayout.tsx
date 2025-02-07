import { organizationsPermissions } from "api/queries/organizations";
import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "components/Breadcrumb/Breadcrumb";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import NotFoundPage from "pages/404Page/404Page";
import { type FC, Suspense, createContext, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router-dom";
import {
	canViewOrganization,
	type OrganizationPermissions,
} from "./organizationPermissions";

export const OrganizationSettingsContext = createContext<
	OrganizationSettingsValue | undefined
>(undefined);

type OrganizationSettingsValue = Readonly<{
	organizations: readonly Organization[];
	permissionsByOrganizationId: Record<string, OrganizationPermissions>;
	organization?: Organization;
	organizationPermissions?: OrganizationPermissions;
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

const OrganizationSettingsLayout: FC = () => {
	const { organizations, canViewOrganizationSettings } = useDashboard();
	const { organization: orgName } = useParams() as {
		organization?: string;
	};

	const organization = orgName
		? organizations.find((org) => org.name === orgName)
		: undefined;

	const orgPermissionsQuery = useQuery(
		organizationsPermissions(organizations?.map((o) => o.id)),
	);

	if (orgPermissionsQuery.isLoading) {
		return <Loader />;
	}

	if (!orgPermissionsQuery.data) {
		return <ErrorAlert error={orgPermissionsQuery.error} />;
	}

	const viewableOrganizations = organizations.filter((org) =>
		canViewOrganization(orgPermissionsQuery.data?.[org.id]),
	);

	// It's currently up to each individual page to show an empty state if there
	// is no matching organization. This is weird and we should probably fix it
	// eventually, but if we handled it here it would break the /new route, and
	// refactoring to fix _that_ is a non-trivial amount of work.
	const organizationPermissions =
		organization && orgPermissionsQuery.data?.[organization.id];
	if (organization && !canViewOrganization(organizationPermissions)) {
		return <NotFoundPage />;
	}

	return (
		<RequirePermission isFeatureVisible={canViewOrganizationSettings}>
			<OrganizationSettingsContext.Provider
				value={{
					organizations: viewableOrganizations,
					permissionsByOrganizationId: orgPermissionsQuery.data,
					organization,
					organizationPermissions,
				}}
			>
				<div>
					<Breadcrumb>
						<BreadcrumbList>
							<BreadcrumbItem>
								<BreadcrumbPage>Admin Settings</BreadcrumbPage>
							</BreadcrumbItem>
							<BreadcrumbSeparator />
							<BreadcrumbItem>
								<BreadcrumbPage className="flex items-center gap-2">
									Organizations
								</BreadcrumbPage>
							</BreadcrumbItem>
							{organization && (
								<>
									<BreadcrumbSeparator />
									<BreadcrumbItem>
										<BreadcrumbPage className="text-content-primary">
											<Avatar
												key={organization.id}
												size="sm"
												fallback={organization.display_name}
												src={organization.icon}
											/>
											{organization.display_name}
										</BreadcrumbPage>
									</BreadcrumbItem>
								</>
							)}
						</BreadcrumbList>
					</Breadcrumb>
					<hr className="h-px border-none bg-border" />
					<div className="px-10 max-w-screen-2xl">
						<Suspense fallback={<Loader />}>
							<Outlet />
						</Suspense>
					</div>
				</div>
			</OrganizationSettingsContext.Provider>
		</RequirePermission>
	);
};

export default OrganizationSettingsLayout;
