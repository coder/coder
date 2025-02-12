import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	type FC,
	type PropsWithChildren,
	createContext,
	useContext,
} from "react";
import { Navigate, Outlet, useParams } from "react-router-dom";

export const GroupsPageContext = createContext<
	OrganizationSettingsValue | undefined
>(undefined);

type OrganizationSettingsValue = Readonly<{
	organization?: Organization;
	showOrganizations: boolean;
}>;

export const useGroupsSettings = (): OrganizationSettingsValue => {
	const context = useContext(GroupsPageContext);
	if (!context) {
		throw new Error(
			"useGroupsSettings should be used inside of GroupsPageContext",
		);
	}

	return context;
};

const GroupsPageProvider: FC = () => {
	const { organizations, showOrganizations } = useDashboard();
	const { organization: orgName } = useParams() as {
		organization?: string;
	};

	const organization = orgName
		? organizations.find((org) => org.name === orgName)
		: getOrganizationByDefault(organizations);

	if (
		location.pathname.startsWith("/deployment/groups") &&
		showOrganizations &&
		organization
	) {
		return (
			<Navigate to={`/organizations/${organization.name}/groups`} replace />
		);
	}

	return (
		<GroupsPageContext.Provider value={{ organization, showOrganizations }}>
			<Outlet />
		</GroupsPageContext.Provider>
	);
};

export default GroupsPageProvider;

const getOrganizationByDefault = (organizations: readonly Organization[]) => {
	return organizations.find((org) => org.is_default);
};
