import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import LogsSidebarView from "./LogsSidebarView";

/**
 * A sidebar for the logs section. Derives page visibility from
 * entitlement features and site-wide permissions.
 */
export const LogsSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();

	return (
		<LogsSidebarView
			canViewAuditLog={
				featureVisibility.audit_log && permissions.viewAnyAuditLog
			}
			canViewConnectionLog={
				featureVisibility.connection_log && permissions.viewAnyConnectionLog
			}
			canViewAIBridge={
				featureVisibility.aibridge && permissions.viewAnyAIBridgeInterception
			}
		/>
	);
};
