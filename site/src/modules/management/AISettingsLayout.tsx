import { type FC, Suspense } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { AISidebar } from "./AISidebar";

const AISettingsLayout: FC = () => {
	const location = useLocation();

	if (location.pathname === "/ai") {
		return <Navigate to="/ai/agents" replace />;
	}

	return (
		<div className="flex flex-row min-h-screen">
			<div className="border-0 border-r border-solid border-border">
				<CollapsibleSidebar storageKey="ai-sidebar-width">
					<AISidebar />
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

export default AISettingsLayout;
