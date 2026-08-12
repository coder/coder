import type { FC } from "react";
import { Navigate } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { canViewWorkspaces } from "#/modules/permissions";

/**
 * Redirects the dashboard index route to the workspaces page, or to account
 * settings when the user cannot read workspaces.
 */
export const DashboardRedirect: FC = () => {
	const { permissions } = useAuthenticated();

	return (
		<Navigate
			to={canViewWorkspaces(permissions) ? "/workspaces" : "/settings/account"}
			replace
		/>
	);
};
