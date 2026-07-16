import {
	BotIcon,
	PanelLeftIcon,
	ShieldCheckIcon,
	StoreIcon,
} from "lucide-react";
import { type FC, useCallback, useEffect, useState } from "react";
import { Link, NavLink } from "react-router";
import { SettingsSidebarNavItem as SidebarNavItem } from "#/components/Sidebar/Sidebar";
import { SidebarAccordion } from "#/components/Sidebar/SidebarAccordion";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { Permissions } from "#/modules/permissions";
import { cn } from "#/utils/cn";
import type { AISection } from "./useActiveAISection";

interface AISettingsSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
	/** Which section is active based on the current route. */
	activeSection: AISection;
}

interface TopLevelNavItemProps {
	label: string;
	href: string;
	icon: FC<{ className?: string }>;
	active: boolean;
}

/**
 * A flat icon+label link. In collapsed mode it renders as an icon
 * with a tooltip; clicking it navigates and re-expands the sidebar.
 */
const TopLevelNavItem: FC<TopLevelNavItemProps> = ({
	label,
	href,
	icon: Icon,
	active,
}) => {
	const { collapsed, expand } = useSidebarContext();

	if (collapsed) {
		return (
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						<Link
							to={href}
							onClick={expand}
							className="flex items-center justify-center w-10 h-10 rounded-md no-underline hover:bg-surface-secondary"
						>
							<Icon
								className={cn(
									"size-4 flex-shrink-0 text-content-secondary",
									active && "text-content-primary",
								)}
							/>
						</Link>
					</TooltipTrigger>
					<TooltipContent side="right">{label}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	return (
		<NavLink
			to={href}
			className={({ isActive }) =>
				cn(
					"flex items-center gap-2 px-3 py-2 h-10 rounded-md no-underline text-sm font-medium text-content-secondary hover:bg-surface-secondary transition-colors",
					isActive && "text-content-primary font-semibold",
				)
			}
		>
			<Icon className="size-4 flex-shrink-0" />
			{label}
		</NavLink>
	);
};

/**
 * Displays navigation for the AI settings section. Providers renders
 * as a flat icon+label link, while the AI Governance and Coder Agents
 * sections use accordions with sub-items.
 */
const AISettingsSidebarView: FC<AISettingsSidebarViewProps> = ({
	permissions,
	activeSection,
}) => {
	const { collapsed, toggle } = useSidebarContext();

	const [agentsOpen, setAgentsOpen] = useState(
		() => activeSection === "coder-agents",
	);
	const governanceActive =
		activeSection === "governance" || activeSection === "gateway-keys";
	const [governanceOpen, setGovernanceOpen] = useState(governanceActive);

	// When navigation changes the active section, open only the
	// accordion that owns the active route.
	useEffect(() => {
		setAgentsOpen(activeSection === "coder-agents");
		setGovernanceOpen(
			activeSection === "governance" || activeSection === "gateway-keys",
		);
	}, [activeSection]);

	const toggleAgents = useCallback(() => {
		setAgentsOpen((prev) => !prev);
	}, []);

	const toggleGovernance = useCallback(() => {
		setGovernanceOpen((prev) => !prev);
	}, []);

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
					<span className="text-sm text-content-secondary">AI</span>
				)}
				<PanelLeftIcon
					className={cn(
						"size-4 text-content-secondary group-hover:text-content-primary transition-colors",
						!collapsed && "ml-auto",
					)}
				/>
			</button>

			{(permissions.viewDeploymentConfig || permissions.viewAIGatewayKeys) && (
				<SidebarAccordion
					icon={ShieldCheckIcon}
					label="AI Governance"
					href={
						permissions.viewDeploymentConfig
							? "/ai/settings/governance"
							: "/ai/settings/gateway-keys"
					}
					open={governanceOpen}
					onToggle={toggleGovernance}
					active={governanceActive}
				>
					<div className="flex flex-col gap-1">
						{permissions.viewDeploymentConfig && (
							<SidebarNavItem href="/ai/settings/governance">
								AI Gateway
							</SidebarNavItem>
						)}
						{permissions.viewAIGatewayKeys && (
							<SidebarNavItem href="/ai/settings/gateway-keys">
								AI Gateway keys
							</SidebarNavItem>
						)}
					</div>
				</SidebarAccordion>
			)}
			{permissions.viewAnyAIProvider && (
				<TopLevelNavItem
					label="Providers"
					href="/ai/settings/providers"
					icon={StoreIcon}
					active={activeSection === "providers"}
				/>
			)}

			{permissions.editDeploymentConfig && (
				<SidebarAccordion
					icon={BotIcon}
					label="Coder Agents"
					href="/ai/settings/coder-agents"
					open={agentsOpen}
					onToggle={toggleAgents}
					active={activeSection === "coder-agents"}
				>
					<div className="flex flex-col gap-1">
						<SidebarNavItem href="/ai/settings/coder-agents">
							General
						</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/models">Models</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/mcp-servers">
							MCP servers
						</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/templates">
							Templates
						</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/spend">Spend</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/instructions">
							Instructions
						</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/lifecycle">
							Lifecycle
						</SidebarNavItem>
					</div>
				</SidebarAccordion>
			)}
		</div>
	);
};

export default AISettingsSidebarView;
