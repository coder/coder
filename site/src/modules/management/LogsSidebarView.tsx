import { BotMessageSquareIcon, CableIcon, ScrollTextIcon } from "lucide-react";
import type { FC } from "react";
import { Sidebar, SidebarNavItem } from "#/components/Sidebar";

interface LogsSidebarViewProps {
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewAIBridge: boolean;
}

/**
 * Displays navigation for the logs section: audit, connection, and
 * AI session logs.
 */
const LogsSidebarView: FC<LogsSidebarViewProps> = ({
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
}) => {
	return (
		<Sidebar>
			<div className="flex flex-col gap-1">
				{canViewAuditLog && (
					<SidebarNavItem icon={ScrollTextIcon} href="/logs/audit">
						Audit logs
					</SidebarNavItem>
				)}
				{canViewConnectionLog && (
					<SidebarNavItem icon={CableIcon} href="/logs/connection">
						Connection logs
					</SidebarNavItem>
				)}
				{canViewAIBridge && (
					<SidebarNavItem icon={BotMessageSquareIcon} href="/logs/ai-sessions">
						AI session logs
					</SidebarNavItem>
				)}
			</div>
		</Sidebar>
	);
};

export default LogsSidebarView;
