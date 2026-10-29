import { useContext } from "react";
import type { User } from "#/api/typesGenerated";
import { DashboardContext, type DashboardValue } from "./DashboardProvider";

export const useDashboard = (): DashboardValue => {
	const context = useContext(DashboardContext);

	if (!context) {
		throw new Error("useDashboard must be used within DashboardProvider");
	}

	return context;
};

export const getDefaultOrganizationName = (
	organizations: DashboardValue["organizations"],
): string => organizations.find((org) => org.is_default)?.name ?? "";

export const getDefaultOrganizationId = (
	organizations: DashboardValue["organizations"],
): string | undefined => organizations.find((org) => org.is_default)?.id;

/**
 * Keeps the dashboard organizations the user belongs to, in dashboard order.
 * The dashboard lists every organization the user can read, but chats only
 * run in member ones.
 */
export const getMemberOrganizations = (
	organizations: DashboardValue["organizations"],
	user: Pick<User, "organization_ids">,
): DashboardValue["organizations"] =>
	organizations.filter((org) => user.organization_ids.includes(org.id));
