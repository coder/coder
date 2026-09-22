import {
	BugIcon,
	GlobeIcon,
	LayoutGridIcon,
	MonitorIcon,
	PlusIcon,
	SquareTerminalIcon,
} from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import type {
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
	icon: ReactNode;
}[] = [
	{ id: "browser", label: "Browser", icon: <GlobeIcon /> },
	{ id: "desktop", label: "Desktop", icon: <MonitorIcon /> },
	{ id: "debug", label: "Debug", icon: <BugIcon /> },
];

// usePortsData requires a workspace and agent, which are optional props on the
// parent control, so the hook lives in this conditionally rendered component.
const AgentPortsSubMenu: FC<{
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

export const RightPanelAddTabControl: FC<{
	workspace?: Workspace;
	agent?: WorkspaceAgent;
	host?: string;
	isRunning?: boolean;
	supportedSingletonTabs: readonly SingletonRightPanelTabId[];
	visibleSingletonTabs: readonly SingletonRightPanelTabId[];
	onToggleSingletonTab: (tabId: SingletonRightPanelTabId) => void;
	onNewTerminal: () => void;
	onOpenWorkspaceApp?: (app: WorkspaceApp) => void;
	onOpenCommandApp?: (app: WorkspaceApp) => void;
	onOpenPort?: (selection: PortSelection) => void;
}> = ({
	workspace,
	agent,
	host = "",
	isRunning = false,
	supportedSingletonTabs,
	visibleSingletonTabs,
	onToggleSingletonTab,
	onNewTerminal,
	onOpenWorkspaceApp,
	onOpenCommandApp,
	onOpenPort,
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
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Add panel"
					title="Add panel"
					className="size-7 shrink-0 text-content-secondary hover:text-content-primary"
				>
					<PlusIcon className="size-3.5" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
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
							if (app.command && onOpenCommandApp) {
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
							if (isWorkspaceAppEmbeddable(app) && onOpenWorkspaceApp) {
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

				{workspace &&
					agent &&
					onOpenPort &&
					canShowPortForwarding(agent, host) && (
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
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
