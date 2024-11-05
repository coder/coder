import { useAuthenticated } from "contexts/auth/RequireAuth";
import type { FC } from "react";
import { DeploymentSidebarView } from "./DeploymentSidebarView";

/**
 * A sidebar for deployment settings.
 */
export const DeploymentSidebar: FC = () => {
	const { permissions } = useAuthenticated();

	return <DeploymentSidebarView permissions={permissions} />;
};
