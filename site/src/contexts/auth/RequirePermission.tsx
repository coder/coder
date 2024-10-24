import type { FC, ReactNode } from "react";
import { Navigate } from "react-router-dom";

export interface RequirePermissionProps {
	children?: ReactNode;
	permitted: boolean;
	unpermittedRedirect?: `/${string}`;
}

/**
 * Wraps routes that are available based on RBAC or licensing.
 */
export const RequirePermission: FC<RequirePermissionProps> = ({
	children,
	permitted,
	unpermittedRedirect = "/workspaces",
}) => {
	if (!permitted) {
		return <Navigate to={unpermittedRedirect} replace />;
	}

	return <>{children}</>;
};
