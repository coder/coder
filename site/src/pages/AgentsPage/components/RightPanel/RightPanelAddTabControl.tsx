import {
	ChevronDownIcon,
	LayoutGridIcon,
	PlusIcon,
	SquareTerminalIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { isWorkspaceAppEmbeddable } from "#/modules/apps/apps";
import { AppLink } from "#/modules/resources/AppLink/AppLink";
import {
	canShowPortForwarding,
	usePortsData,
} from "#/modules/resources/usePortsData";
import { cn } from "#/utils/cn";
import type { PortSelection } from "../../utils/rightPanelTabs";
import { PortsMenuItem } from "../WorkspacePillPorts";

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
	onNewTerminal: () => void;
	onOpenWorkspaceApp?: (app: WorkspaceApp) => void;
	onOpenCommandApp?: (app: WorkspaceApp) => void;
	onOpenPort?: (selection: PortSelection) => void;
}> = ({
	workspace,
	agent,
	host = "",
	isRunning = false,
	onNewTerminal,
	onOpenWorkspaceApp,
	onOpenCommandApp,
	onOpenPort,
}) => {
	const [open, setOpen] = useState(false);
	const userApps = agent?.apps.filter((app) => !app.hidden) ?? [];
	const canCreateTerminal =
		workspace !== undefined && agent !== undefined && isRunning;

	return (
		<div className="flex h-6 shrink-0 items-center overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary text-content-secondary">
			<Button
				variant="subtle"
				size="icon"
				onClick={onNewTerminal}
				disabled={!canCreateTerminal}
				aria-label="New terminal tab"
				title="New terminal tab"
				className="size-6 rounded-none border-0 bg-transparent p-0 text-content-secondary hover:bg-surface-secondary hover:text-content-primary border-r border-solid border-border-default"
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
					className="w-52 p-1 [&_[role=menuitem]]:py-1 [&_[role=menuitem]]:text-xs [&_img]:!size-3.5 [&_svg]:!size-3.5"
				>
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
		</div>
	);
};
