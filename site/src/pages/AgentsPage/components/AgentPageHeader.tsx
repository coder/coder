import {
	BarChart3Icon,
	ChevronLeftIcon,
	PanelLeftIcon,
	SettingsIcon,
} from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC, ReactNode } from "react";
import { Link, NavLink, useLocation, useOutletContext } from "react-router";
import { cn } from "utils/cn";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { CoderIcon } from "#/components/Icons/CoderIcon";
import type { AgentsOutletContext } from "../AgentsPageView";
import { sidebarViewFromPath } from "./Sidebar/AgentsSidebar";

interface AgentPageHeaderProps {
	children?: ReactNode;
	/** When set, shows a back link on mobile instead of the logo
	 *  and hides the settings/analytics nav buttons. */
	mobileBack?: { to: string; label: string };
}

export const AgentPageHeader: FC<AgentPageHeaderProps> = ({
	children,
	mobileBack,
}) => {
	const { isSidebarCollapsed, onExpandSidebar } =
		useOutletContext<AgentsOutletContext>();
	const { appearance } = useDashboard();
	const logoUrl = appearance.logo_url;
	const location = useLocation();
	const sidebarView = sidebarViewFromPath(location.pathname);

	return (
		<div className="flex shrink-0 items-center gap-2 px-4 pt-3 pb-0.5 md:py-0.5">
			{mobileBack ? (
				<Link
					to={mobileBack.to}
					className="inline-flex shrink-0 items-center gap-1 text-sm text-content-secondary no-underline hover:text-content-primary md:hidden"
				>
					<ChevronLeftIcon className="h-4 w-4" />
					{mobileBack.label}
				</Link>
			) : (
				<NavLink to="/workspaces" className="inline-flex shrink-0 md:hidden">
					{logoUrl ? (
						<ExternalImage className="h-6" src={logoUrl} alt="Logo" />
					) : (
						<CoderIcon className="h-6 w-6 fill-content-primary" />
					)}
				</NavLink>
			)}
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
			{/* Mobile-only nav buttons mirroring the sidebar toolbar
			 * which is hidden below the md breakpoint. */}
			{!mobileBack && (
				<div className="flex items-center gap-0.5 md:hidden">
					<Button
						asChild
						variant="subtle"
						size="icon"
						aria-label="Settings"
						className={cn(
							"h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary",
							sidebarView.panel === "settings" && "text-content-primary",
						)}
					>
						<Link to="/agents/settings" state={{ from: location.pathname }}>
							<SettingsIcon />
						</Link>
					</Button>
					<Button
						asChild
						variant="subtle"
						size="icon"
						aria-label="Analytics"
						className={cn(
							"h-7 w-7 min-w-0 text-content-secondary hover:text-content-primary",
							sidebarView.panel === "analytics" && "text-content-primary",
						)}
					>
						<Link to="/agents/analytics">
							<BarChart3Icon />
						</Link>
					</Button>
				</div>
			)}{" "}
			{children && <div className="flex items-center gap-2">{children}</div>}
		</div>
	);
};
