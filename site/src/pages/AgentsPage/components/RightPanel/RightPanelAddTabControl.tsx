import { cn } from "cn";
import {
	AppWindowIcon,
	BugIcon,
	ChevronDownIcon,
	GlobeIcon,
	LayoutGridIcon,
	MonitorIcon,
	PlusIcon,
	SquareTerminalIcon,
} from "lucide-react";
import { useState } from "react";
import { useQuery } from "react-query";
import { chatMCPAppEntrypoints } from "#/api/queries/chats";
import type {
	ChatMCPAppEntrypoint,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	AGENT_BROWSER_APP_SLUG,
	isWorkspaceAppEmbeddable,
} from "#/modules/apps/apps";
import { AppLink } from "#/modules/resources/AppLink/AppLink";
import {
	canShowPortForwarding,
	usePortsData,
} from "#/modules/resources/usePortsData";
import type {
	PortSelection,
	SingletonRightPanelTabId,
} from "../../utils/rightPanelTabs";
import { PortsMenuItem } from "../WorkspacePillPorts";

const singletonTabMenuEntries: readonly {
	id: SingletonRightPanelTabId;
	label: string;
	icon: React.ReactNode;
}[] = [
	{ id: "browser", label: "Browser", icon: <GlobeIcon /> },
	{ id: "desktop", label: "Desktop", icon: <MonitorIcon /> },
	{ id: "debug", label: "Debug", icon: <BugIcon /> },
];

// usePortsData requires a workspace and agent, which are optional props on the
// parent control, so the hook lives in this conditionally rendered component.
const AgentPortsSubMenu: React.FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	isOpen: boolean;
	isRunning: boolean;
	onPortSelect: (selection: PortSelection) => void;
}> = ({ workspace, agent, host, isOpen, isRunning, onPortSelect }) => {
	const portsData = usePortsData(
		workspace,
		agent,
		isOpen && agent.status === "connected",
	);
	return (
		<PortsMenuItem
			workspace={workspace}
			agent={agent}
			host={host}
			portsData={portsData}
			isRunning={isRunning}
			isBelowMd={false}
			focusOnMount={false}
			onPortSelect={onPortSelect}
		/>
	);
};

export type McpAppsMenuSource = {
	chatId: string;
	/** The MCP servers selected in the composer. */
	mcpServerIds: readonly string[];
	onOpen: (entrypoint: ChatMCPAppEntrypoint) => void;
};

// Lists the MCP Apps thread entry points of the selected MCP servers. It
// mounts only while the menu is open, so coderd connects to the servers
// only then.
const McpAppsMenuItems: React.FC<McpAppsMenuSource> = ({
	chatId,
	mcpServerIds,
	onOpen,
}) => {
	const entrypointsQuery = useQuery(
		chatMCPAppEntrypoints(chatId, mcpServerIds),
	);
	if (entrypointsQuery.isError) {
		return null;
	}
	const entrypoints = entrypointsQuery.data;
	if (entrypoints && entrypoints.length === 0) {
		return null;
	}
	return (
		<>
			<DropdownMenuSeparator className="my-1" />
			<DropdownMenuLabel className="px-2 py-1 text-2xs text-content-secondary">
				MCP apps
			</DropdownMenuLabel>
			{entrypoints === undefined ? (
				<DropdownMenuItem disabled>Loading MCP apps...</DropdownMenuItem>
			) : (
				entrypoints.map((entrypoint) => (
					<DropdownMenuItem
						key={`${entrypoint.mcp_server_config_id}:${entrypoint.tool_name}`}
						onSelect={() => onOpen(entrypoint)}
					>
						<AppWindowIcon />
						<span className="truncate">{entrypoint.title}</span>
						<span className="ml-auto truncate text-content-secondary">
							{entrypoint.server_display_name}
						</span>
					</DropdownMenuItem>
				))
			)}
		</>
	);
};

export const RightPanelAddTabControl: React.FC<{
	workspace?: Workspace;
	agent?: WorkspaceAgent;
	host: string;
	isRunning: boolean;
	supportedSingletonTabs: readonly SingletonRightPanelTabId[];
	visibleSingletonTabs: readonly SingletonRightPanelTabId[];
	onToggleSingletonTab: (tabId: SingletonRightPanelTabId) => void;
	onNewTerminal: () => void;
	onOpenWorkspaceApp: (app: WorkspaceApp) => void;
	onOpenCommandApp: (app: WorkspaceApp) => void;
	onOpenPort: (selection: PortSelection) => void;
	/** Set when MCP Apps views can open from this menu. */
	mcpApps?: McpAppsMenuSource;
}> = ({
	workspace,
	agent,
	host,
	isRunning,
	supportedSingletonTabs,
	visibleSingletonTabs,
	onToggleSingletonTab,
	onNewTerminal,
	onOpenWorkspaceApp,
	onOpenCommandApp,
	onOpenPort,
	mcpApps,
}) => {
	const [open, setOpen] = useState(false);
	// agent-browser already has the built-in Browser tab.
	const userApps =
		agent?.apps.filter(
			(app) => !app.hidden && app.slug !== AGENT_BROWSER_APP_SLUG,
		) ?? [];
	const canCreateTerminal =
		workspace !== undefined && agent !== undefined && isRunning;
	const singletonEntries = singletonTabMenuEntries.filter((entry) =>
		supportedSingletonTabs.includes(entry.id),
	);

	return (
		<div className="flex h-6 shrink-0 items-center overflow-hidden rounded-md border border-solid border-border bg-surface-primary text-content-secondary">
			<Button
				variant="subtle"
				size="icon"
				onClick={onNewTerminal}
				disabled={!canCreateTerminal}
				aria-label="New terminal tab"
				title="New terminal tab"
				className="size-6 rounded-none border-0 bg-transparent p-0 text-content-secondary hover:bg-surface-secondary hover:text-content-primary border-r border-solid border-border"
			>
				<PlusIcon className="size-3.5" />
			</Button>
			<DropdownMenu open={open} onOpenChange={setOpen}>
				<DropdownMenuTrigger asChild>
					<Button
						variant="subtle"
						size="icon"
						aria-label="Add panel"
						className="size-6 rounded-none border-0 bg-transparent p-0 text-content-secondary hover:bg-surface-secondary hover:text-content-primary"
					>
						<ChevronDownIcon
							className={cn(
								"size-3 transition-transform",
								open && "rotate-180",
							)}
						/>
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent
					align="end"
					side="bottom"
					className="w-52 p-1 [&_[role^=menuitem]]:py-1 [&_[role^=menuitem]]:text-xs [&_img]:size-3.5! [&_svg]:size-3.5!"
				>
					{singletonEntries.length > 0 && (
						<>
							{singletonEntries.map((entry) => (
								<DropdownMenuCheckboxItem
									key={entry.id}
									checked={visibleSingletonTabs.includes(entry.id)}
									onSelect={() => onToggleSingletonTab(entry.id)}
								>
									{entry.icon}
									{entry.label}
								</DropdownMenuCheckboxItem>
							))}
							<DropdownMenuSeparator className="my-1" />
						</>
					)}

					<DropdownMenuItem
						onSelect={onNewTerminal}
						disabled={!canCreateTerminal}
					>
						<SquareTerminalIcon />
						New Terminal
					</DropdownMenuItem>

					{workspace && agent && userApps.length > 0 && (
						<>
							<DropdownMenuSeparator className="my-1" />
							{userApps.map((app) => {
								if (app.command) {
									return (
										<DropdownMenuItem
											key={app.id}
											onSelect={() => onOpenCommandApp(app)}
											disabled={!isRunning}
										>
											{app.icon ? (
												<ExternalImage
													src={app.icon}
													alt=""
													className="rounded-sm"
												/>
											) : (
												<SquareTerminalIcon />
											)}
											{app.display_name ?? app.slug}
										</DropdownMenuItem>
									);
								}
								if (isWorkspaceAppEmbeddable(app)) {
									return (
										<DropdownMenuItem
											key={app.id}
											onSelect={() => onOpenWorkspaceApp(app)}
											disabled={!isRunning}
										>
											{app.icon ? (
												<ExternalImage
													src={app.icon}
													alt=""
													className="rounded-sm"
												/>
											) : (
												<LayoutGridIcon />
											)}
											{app.display_name ?? app.slug}
										</DropdownMenuItem>
									);
								}
								return (
									<AppLink
										key={app.id}
										workspace={workspace}
										agent={agent}
										app={app}
										grouped
									/>
								);
							})}
						</>
					)}

					{workspace && agent && canShowPortForwarding(agent, host) && (
						<>
							<DropdownMenuSeparator className="my-1" />
							<AgentPortsSubMenu
								workspace={workspace}
								agent={agent}
								host={host}
								isOpen={open}
								isRunning={isRunning}
								onPortSelect={onOpenPort}
							/>
						</>
					)}

					{open && mcpApps && mcpApps.mcpServerIds.length > 0 && (
						<McpAppsMenuItems {...mcpApps} />
					)}
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	);
};
