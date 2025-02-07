import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbLink,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "components/Breadcrumb/Breadcrumb";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, Suspense, createContext, useContext } from "react";
import { Outlet, useParams } from "react-router-dom";

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

	const organization = orgName
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
											{organization?.name}
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
