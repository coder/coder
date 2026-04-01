import type { FC } from "react";
import { Outlet, useLocation } from "react-router";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { AgentPageHeader } from "./components/AgentPageHeader";

const AgentSettingsPage: FC = () => {
	const location = useLocation();
	const match = location.pathname.match(/\/agents\/settings\/(.+)/);
	const section = match?.[1];

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:!block">
			<AgentPageHeader
				mobileBack={
					section ? { to: "/agents/settings", label: "Settings" } : undefined
				}
			/>
			<div className="p-4 pt-8">
				<div className="mx-auto w-full max-w-3xl">
					<Outlet />
				</div>
			</div>
		</ScrollArea>
	);
};

export default AgentSettingsPage;
