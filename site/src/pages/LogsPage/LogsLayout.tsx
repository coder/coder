import { Navigate } from "react-router";
import { SidebarLayout } from "#/components/Sidebar";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { LogsSidebar } from "#/modules/management/LogsSidebar";

/**
 * Redirects `/logs` to the first log page the user can view, matching
 * the AI settings index redirect pattern.
 */
export const LogsIndexRedirect = () => {
	const { permissions } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();

	if (featureVisibility.audit_log && permissions.viewAnyAuditLog) {
		return <Navigate to="/logs/audit" replace />;
	}

	if (featureVisibility.connection_log && permissions.viewAnyConnectionLog) {
		return <Navigate to="/logs/connection" replace />;
	}

	if (featureVisibility.aibridge && permissions.viewAnyAIBridgeInterception) {
		return <Navigate to="/logs/ai-sessions" replace />;
	}

	return <Navigate to="/" replace />;
};

const LogsLayout = () => {
	return <SidebarLayout sidebar={<LogsSidebar />} />;
};

export default LogsLayout;
