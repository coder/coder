import type { FC } from "react";
import { useQuery } from "react-query";
import {
	Outlet,
	useLocation,
	useNavigate,
	useSearchParams,
} from "react-router";
import { organizationsPermissions } from "#/api/queries/organizations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import NotFoundPage from "#/pages/NotFoundPage/NotFoundPage";
import {
	modelOrganizationSearchParam,
	OrganizationModelsContext,
	selectModelOrganization,
	useAccessibleModelOrganizations,
} from "./organizationModels";

const OrganizationModelsLayout: FC = () => {
	const { organizations } = useDashboard();
	const location = useLocation();
	const navigate = useNavigate();
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
		return <NotFoundPage />;
	}

	if (permissionsQuery.data === undefined && permissionsQuery.error) {
		return <ErrorAlert error={permissionsQuery.error} />;
	}

	if (permissionsQuery.data === undefined) {
		return <Loader />;
	}

	if (!activePermissions) {
		return <NotFoundPage />;
	}

	return (
		<OrganizationModelsContext.Provider
			value={{
				organization: activeOrganization,
				permissions: activePermissions,
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
				{accessibleOrganizations.length > 1 && (
					<div>
						<OrganizationAutocomplete
							value={activeOrganization}
							ariaLabel={`Organization ${getOrganizationLabel(
								activeOrganization,
								accessibleOrganizations,
							)}`}
							options={accessibleOrganizations}
							triggerClassName="w-60"
							optionsTabbable
							onChange={(organization) => {
								if (!organization) {
									return;
								}
								const next = new URLSearchParams(searchParams);
								next.set(modelOrganizationSearchParam, organization.name);
								void navigate(`${location.pathname}?${next.toString()}`);
							}}
						/>
					</div>
				)}
				<Outlet />
			</div>
		</OrganizationModelsContext.Provider>
	);
};

export default OrganizationModelsLayout;
