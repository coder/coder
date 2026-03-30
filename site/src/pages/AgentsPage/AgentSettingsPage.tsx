import type { FC } from "react";
import { Outlet, useLocation } from "react-router";
import { AgentPageHeader } from "./components/AgentPageHeader";

const AgentSettingsPage: FC = () => {
	const location = useLocation();
	const match = location.pathname.match(/\/agents\/settings\/(.+)/);
	const section = match?.[1];

	return (
		<>
			<AgentPageHeader
				mobileBack={
					section ? { to: "/agents/settings", label: "Settings" } : undefined
				}
			/>
			<div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-4 pt-8 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
				<div className="mx-auto w-full max-w-3xl">
					<Outlet />
				</div>
			</div>
		</>
	);
};

export default AgentSettingsPage;
