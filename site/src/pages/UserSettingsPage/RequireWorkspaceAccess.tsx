import type { FC } from "react";
import { Outlet } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { canViewWorkspaces } from "#/modules/permissions";
import { RequirePermission } from "#/modules/permissions/RequirePermission";

export const RequireWorkspaceAccess: FC = () => {
	const { permissions } = useAuthenticated();

	return (
		<RequirePermission isFeatureVisible={canViewWorkspaces(permissions)}>
			<Outlet />
		</RequirePermission>
	);
};
