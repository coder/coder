import {
	Building2Icon,
	CpuIcon,
	Link2Icon,
	PanelLeftIcon,
	ShieldCheckIcon,
	StoreIcon,
} from "lucide-react";
import { type FC, useCallback, useEffect, useState } from "react";
import { Link, NavLink } from "react-router";
import { SettingsSidebarNavItem } from "#/components/Sidebar/Sidebar";
import { SidebarAccordion } from "#/components/Sidebar/SidebarAccordion";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import type { AISection } from "./useActiveAISection";

interface AISidebarViewProps {
	/** Which section is active based on the current route. */
	activeSection: AISection;
}

interface TopLevelNavItem {
	label: string;
	href: string;
	icon: FC<{ className?: string }>;
	section: AISection;
}

const TOP_LEVEL_ITEMS: TopLevelNavItem[] = [
	{
		label: "AI Governance",
		href: "/ai/governance",
		icon: ShieldCheckIcon,
		section: "ai-governance",
	},
	{
		label: "Providers",
		href: "/ai/providers",
		icon: StoreIcon,
		section: "providers",
	},
	{
		label: "Models",
		href: "/ai/models",
		icon: CpuIcon,
		section: "models",
	},
	{
		label: "Spend",
		href: "/ai/spend",
		icon: Link2Icon,
		section: "spend",
	},
];

/**
 * Displays navigation for the AI settings section. Top-level items
 * are rendered as flat icon+label links, while the Agents section
 * uses an accordion with sub-items.
 */
export const AISidebarView: FC<AISidebarViewProps> = ({ activeSection }) => {
	const { collapsed, toggle, expand } = useSidebarContext();

	const [agentsOpen, setAgentsOpen] = useState(
		() => activeSection === "agents",
	);

	// When navigation changes the active section, open the agents
	// accordion only when agents is active, close otherwise.
	useEffect(() => {
		setAgentsOpen(activeSection === "agents");
	}, [activeSection]);

	const toggleAgents = useCallback(() => {
		setAgentsOpen((prev) => !prev);
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

			{/* Top-level nav items */}
			{TOP_LEVEL_ITEMS.map((item) => {
				const Icon = item.icon;
				const isActive = activeSection === item.section;

				if (collapsed) {
					return (
						<TooltipProvider key={item.section}>
							<Tooltip delayDuration={0}>
								<TooltipTrigger asChild>
									<Link
										to={item.href}
										onClick={expand}
										className="flex items-center justify-center w-10 h-10 rounded-md no-underline hover:bg-surface-secondary"
									>
										<Icon
											className={cn(
												"size-4 flex-shrink-0 text-content-secondary",
												isActive && "text-content-primary",
											)}
										/>
									</Link>
								</TooltipTrigger>
								<TooltipContent side="right">
									{item.label}
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					);
				}

				return (
					<NavLink
						key={item.section}
						to={item.href}
						className={({ isActive: active }) =>
							cn(
								"flex items-center gap-2 px-3 py-2 h-10 rounded-md no-underline text-sm font-medium text-content-secondary hover:bg-surface-secondary transition-colors",
								active && "text-content-primary font-semibold",
							)
						}
					>
						<Icon className="size-4 flex-shrink-0" />
						{item.label}
					</NavLink>
				);
			})}

			{/* Agents accordion */}
			<SidebarAccordion
				icon={Building2Icon}
				label="Agents"
				href="/ai/agents"
				open={agentsOpen}
				onToggle={toggleAgents}
				active={activeSection === "agents"}
			>
				<div className="flex flex-col gap-1">
					<SettingsSidebarNavItem href="/ai/agents" end>
						General
					</SettingsSidebarNavItem>
					<SettingsSidebarNavItem href="/ai/agents/instructions">
						Instructions
					</SettingsSidebarNavItem>
					<SettingsSidebarNavItem href="/ai/agents/templates">
						Templates
					</SettingsSidebarNavItem>
					<SettingsSidebarNavItem href="/ai/agents/experiments">
						Experiments
					</SettingsSidebarNavItem>
					<SettingsSidebarNavItem href="/ai/agents/mcp-servers">
						MCP servers
					</SettingsSidebarNavItem>
					<SettingsSidebarNavItem href="/ai/agents/lifecycle">
						Lifecycle
					</SettingsSidebarNavItem>
				</div>
			</SidebarAccordion>
		</div>
	);
};
