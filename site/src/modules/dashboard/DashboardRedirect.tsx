import type { FC } from "react";
import { Navigate } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { canViewTemplates, canViewWorkspaces } from "#/modules/permissions";

/**
 * Redirects the dashboard index route to the workspaces page, to the templates
 * page when the user cannot read workspaces, or to account settings when the
 * user can read neither.
 */
export const DashboardRedirect: FC = () => {
	const { permissions } = useAuthenticated();

	let to = "/settings/account";
	if (canViewWorkspaces(permissions)) {
		to = "/workspaces";
	} else if (canViewTemplates(permissions)) {
		to = "/templates";
	}

	return <Navigate to={to} replace />;
};
