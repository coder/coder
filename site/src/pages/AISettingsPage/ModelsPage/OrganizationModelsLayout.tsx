import type { FC } from "react";
import { useQuery } from "react-query";
import { Outlet, useSearchParams } from "react-router";
import { organizationsPermissions } from "#/api/queries/organizations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { NotFound } from "#/components/NotFound/NotFound";
import { useAccessibleModelOrganizations } from "#/modules/aiModels/organizationModels";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	modelOrganizationSearchParam,
	OrganizationModelsContext,
	selectModelOrganization,
} from "./organizationModels";

const OrganizationModelsLayout: FC = () => {
	const { organizations } = useDashboard();
	const [searchParams] = useSearchParams();
	const accessibleOrganizationsQuery =
		useAccessibleModelOrganizations(organizations);
	const accessibleOrganizations = accessibleOrganizationsQuery.organizations;
	const organizationSelection = selectModelOrganization(
		accessibleOrganizations,
		searchParams.get(modelOrganizationSearchParam),
	);
	const activeOrganization = organizationSelection.organization;
	const permissionsQuery = useQuery({
		...organizationsPermissions(
			activeOrganization ? [activeOrganization.id] : undefined,
		),
		enabled: activeOrganization !== undefined,
	});
	const activePermissions = activeOrganization
		? permissionsQuery.data?.[activeOrganization.id]
		: undefined;

	if (accessibleOrganizationsQuery.error) {
		return <ErrorAlert error={accessibleOrganizationsQuery.error} />;
	}

	if (accessibleOrganizationsQuery.isLoading) {
		return <Loader />;
	}

	if (!activeOrganization) {
		return <NotFound />;
	}

	if (permissionsQuery.data === undefined && permissionsQuery.error) {
		return <ErrorAlert error={permissionsQuery.error} />;
	}

	if (permissionsQuery.data === undefined) {
		return <Loader />;
	}

	if (!activePermissions) {
		return <NotFound />;
	}

	return (
		<OrganizationModelsContext.Provider
			value={{
				organization: activeOrganization,
				accessibleOrganizations,
				permissions: activePermissions,
				permissionsByOrganization:
					accessibleOrganizationsQuery.permissionsByOrganization,
				requestedOrganizationDenied:
					organizationSelection.requestedOrganizationDenied,
			}}
		>
			<div className="flex flex-col gap-6">
				{(accessibleOrganizationsQuery.partialError ??
					permissionsQuery.error) != null && (
					<div>
						<ErrorAlert
							error={
								accessibleOrganizationsQuery.partialError ??
								permissionsQuery.error
							}
						/>
					</div>
				)}
				<Outlet />
			</div>
		</OrganizationModelsContext.Provider>
	);
};

export default OrganizationModelsLayout;
