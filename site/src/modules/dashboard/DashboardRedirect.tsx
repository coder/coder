import type { FC } from "react";
import { Navigate } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";

/**
 * Resolves the dashboard index route to a landing page based on the signed-in
 * user's permissions. Users who cannot read workspaces land on their account
 * settings.
 */
export const DashboardRedirect: FC = () => {
	const { permissions } = useAuthenticated();

	return (
		<Navigate
			to={permissions.viewWorkspaces ? "/workspaces" : "/settings/account"}
			replace
		/>
	);
};
