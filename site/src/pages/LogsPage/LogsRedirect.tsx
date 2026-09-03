import type { FC } from "react";
import { Navigate } from "react-router";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { firstVisibleLogsPage } from "#/modules/management/adminNavigation";

/**
 * Redirects /logs to the first log page the user can view.
 */
const LogsRedirect: FC = () => {
	const { permissions } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();

	return (
		<Navigate
			to={firstVisibleLogsPage({
				canViewAuditLog:
					featureVisibility.audit_log && permissions.viewAnyAuditLog,
				canViewConnectionLog:
					featureVisibility.connection_log && permissions.viewAnyConnectionLog,
				canViewAIBridge:
					featureVisibility.aibridge && permissions.viewAnyAIBridgeInterception,
			})}
			replace
		/>
	);
};

export default LogsRedirect;
