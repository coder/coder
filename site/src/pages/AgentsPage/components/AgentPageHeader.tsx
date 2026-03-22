import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import { PanelLeftIcon } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC, ReactNode } from "react";
import { NavLink, useOutletContext } from "react-router";
import type { AgentsOutletContext } from "../AgentsPageView";

interface AgentPageHeaderProps {
	children?: ReactNode;
}

export const AgentPageHeader: FC<AgentPageHeaderProps> = ({ children }) => {
	const { isSidebarCollapsed, onExpandSidebar } =
		useOutletContext<AgentsOutletContext>();
	const { appearance } = useDashboard();
	const logoUrl = appearance.logo_url;

	return (
		<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
			<NavLink to="/workspaces" className="inline-flex shrink-0 md:hidden">
				{logoUrl ? (
					<ExternalImage className="h-6" src={logoUrl} alt="Logo" />
				) : (
					<CoderIcon className="h-6 w-6 fill-content-primary" />
				)}
			</NavLink>
			{isSidebarCollapsed && (
				<Button
					variant="subtle"
					size="icon"
					onClick={onExpandSidebar}
					aria-label="Expand sidebar"
					className="hidden h-7 w-7 min-w-0 shrink-0 md:inline-flex"
				>
					<PanelLeftIcon />
				</Button>
			)}
			<div className="min-w-0 flex-1" />
			{children && <div className="flex items-center gap-2">{children}</div>}
		</div>
	);
};
