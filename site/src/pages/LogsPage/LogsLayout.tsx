import { Suspense } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { LogsSidebar } from "#/modules/management/LogsSidebar";
import { linkToAuditing } from "#/modules/navigation";

const LogsLayout = () => {
	const { permissions } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();
	const location = useLocation();

	if (location.pathname === "/logs") {
		const canViewAuditLog =
			featureVisibility.audit_log && permissions.viewAnyAuditLog;
		const canViewConnectionLog =
			featureVisibility.connection_log && permissions.viewAnyConnectionLog;
		return (
			<Navigate
				to={
					canViewAuditLog
						? linkToAuditing
						: canViewConnectionLog
							? "/connectionlog"
							: "/ai-gateway/sessions"
				}
				replace
			/>
		);
	}

	return (
		<div className="flex flex-row min-h-screen">
			<div className="relative z-30 border-0 border-r border-solid border-border">
				<CollapsibleSidebar storageKey="logs-sidebar-width" overlay peekOnMount>
					<LogsSidebar />
				</CollapsibleSidebar>
			</div>
			<div className="flex-1 min-w-0 pt-6 pb-10 px-10">
				<div className="max-w-screen-2xl mx-auto">
					<Suspense fallback={<Loader />}>
						<Outlet />
					</Suspense>
				</div>
			</div>
		</div>
	);
};

export default LogsLayout;
