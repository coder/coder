import type { FC } from "react";
import { Navigate, Outlet } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";

/**
 * Guards user settings routes that only apply to workspaces. Users who cannot
 * read workspaces are sent to the account page, since those settings have no
 * effect for them.
 */
export const RequireWorkspaceAccess: FC = () => {
	const { permissions } = useAuthenticated();

	if (!permissions.viewWorkspaces) {
		return <Navigate to="/settings/account" replace />;
	}

	return <Outlet />;
};
