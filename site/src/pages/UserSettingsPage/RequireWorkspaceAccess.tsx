import type { FC } from "react";
import { Navigate, Outlet } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { canViewWorkspaces } from "#/modules/permissions";

/**
 * Renders the settings route it wraps, or redirects to the account page when
 * the user cannot read workspaces.
 */
export const RequireWorkspaceAccess: FC = () => {
	const { permissions } = useAuthenticated();

	if (!canViewWorkspaces(permissions)) {
		return <Navigate to="/settings/account" replace />;
	}

	return <Outlet />;
};
