import {
	BotMessageSquareIcon,
	CableIcon,
	PanelLeftIcon,
	ScrollTextIcon,
} from "lucide-react";
import type { FC } from "react";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import { linkToAuditing } from "#/modules/navigation";
import { cn } from "#/utils/cn";
import { SidebarTopLevelNavItem } from "./SidebarTopLevelNavItem";
import type { LogsSection } from "./useActiveLogsSection";

interface LogsSidebarViewProps {
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewAIBridge: boolean;
	/** Which section is active based on the current route. */
	activeSection: LogsSection;
}

/**
 * Displays navigation for the logs section as flat icon+label links.
 */
const LogsSidebarView: FC<LogsSidebarViewProps> = ({
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
	activeSection,
}) => {
	const { collapsed, toggle } = useSidebarContext();

	return (
		<div className="flex flex-col gap-1">
			<button
				type="button"
				onClick={toggle}
				className={cn(
					"group flex items-center bg-transparent border-none cursor-pointer mb-1 p-0",
					collapsed
						? "w-10 h-10 justify-center rounded-md"
						: "w-full px-3 rounded-md h-10",
				)}
			>
				{!collapsed && (
					<span className="text-sm text-content-secondary">Logs</span>
				)}
				<PanelLeftIcon
					className={cn(
						"size-4 text-content-secondary group-hover:text-content-primary transition-colors",
						!collapsed && "ml-auto",
					)}
				/>
			</button>

			{canViewAuditLog && (
				<SidebarTopLevelNavItem
					label="Audit logs"
					href={linkToAuditing}
					icon={ScrollTextIcon}
					active={activeSection === "audit"}
				/>
			)}
			{canViewConnectionLog && (
				<SidebarTopLevelNavItem
					label="Connection logs"
					href="/connectionlog"
					icon={CableIcon}
					active={activeSection === "connection"}
				/>
			)}
			{canViewAIBridge && (
				<SidebarTopLevelNavItem
					label="AI session logs"
					href="/ai-gateway/sessions"
					icon={BotMessageSquareIcon}
					active={activeSection === "ai-sessions"}
				/>
			)}
		</div>
	);
};

export default LogsSidebarView;
