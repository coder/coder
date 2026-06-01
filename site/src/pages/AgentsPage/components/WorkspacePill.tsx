import {
	ArrowLeftIcon,
	BuildingIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	CopyIcon,
	ExternalLinkIcon,
	LayoutGridIcon,
	LockIcon,
	LockOpenIcon,
	MonitorIcon,
	NetworkIcon,
	RadioIcon,
	SquareTerminalIcon,
} from "lucide-react";
import type { FC } from "react";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useMutation, useQuery } from "react-query";
import { Link } from "react-router";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import { workspacePortShares } from "#/api/queries/workspaceportsharing";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentListeningPort,
	WorkspaceAgentPortShare,
	WorkspaceApp,
} from "#/api/typesGenerated";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { VSCodeIcon } from "#/components/Icons/VSCodeIcon";
import { VSCodeInsidersIcon } from "#/components/Icons/VSCodeInsidersIcon";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useProxy } from "#/contexts/ProxyContext";
import { useClipboard } from "#/hooks/useClipboard";
import {
	getTerminalHref,
	getVSCodeHref,
	isExternalApp,
	needsSessionToken,
	openAppInNewWindow,
} from "#/modules/apps/apps";
import { useAppLink } from "#/modules/apps/useAppLink";
import { cn } from "#/utils/cn";
import { isBelowMdViewport } from "#/utils/mobile";
import {
	getWorkspaceListeningPortsProtocol,
	portForwardURL,
} from "#/utils/portForward";
import { getWorkspaceStatus, StatusIcon } from "./StatusIcon";

interface WorkspacePillProps {
	workspace: Workspace;
	agent: WorkspaceAgent;
	chatId: string;
	sshCommand?: string;
	folder?: string;
}

export const WorkspacePill: FC<WorkspacePillProps> = ({
	workspace,
	agent,
	chatId,
	sshCommand,
	folder,
}) => {
	const [open, setOpen] = useState(false);
	const [tooltipOpen, setTooltipOpen] = useState(false);
	const isRunning = workspace.latest_build.status === "running";
	const route = `/@${workspace.owner_name}/${workspace.name}`;

	const { effectiveType, statusLabel } = getWorkspaceStatus(workspace, agent);

	const { mutate: generateKey, isPending: isGeneratingKey } = useMutation({
		mutationFn: () => API.getApiKey(),
	});
	const { proxy } = useProxy();
	const host = proxy.preferredWildcardHostname;

	const builtinApps = new Set(agent.display_apps);
	const hasVSCode = builtinApps.has("vscode");
	const hasVSCodeInsiders = builtinApps.has("vscode_insiders");
	const hasTerminal = builtinApps.has("web_terminal");
	const portForwardingEnabled =
		host !== "" && builtinApps.has("port_forwarding_helper");

	const userApps = agent.apps.filter((app) => !app.hidden);

	const hasItemsAboveSeparator =
		hasVSCode ||
		hasVSCodeInsiders ||
		userApps.length > 0 ||
		hasTerminal ||
		portForwardingEnabled;

	// Flyout sub-menus clip on mobile.
	const [view, setView] = useState<"main" | "ports">("main");
	const [focusPortsOnMain, setFocusPortsOnMain] = useState(false);
	const isBelowMd = useIsBelowMdViewport();
	const showPortsView = view === "ports" && isBelowMd;

	useEffect(() => {
		if (!isBelowMd && view === "ports") {
			setView("main");
			setFocusPortsOnMain(false);
		}
	}, [isBelowMd, view]);

	return (
		<DropdownMenu
			open={open}
			onOpenChange={(next) => {
				setOpen(next);
				if (!next) {
					setView("main");
					setFocusPortsOnMain(false);
				}
			}}
		>
			<Tooltip
				open={tooltipOpen}
				onOpenChange={(v) => setTooltipOpen(v && !open)}
			>
				<TooltipTrigger asChild>
					<DropdownMenuTrigger asChild>
						<button
							type="button"
							aria-label={`${workspace.name} workspace menu`}
							className={cn(
								"inline-flex min-w-0 items-center gap-1 rounded-full bg-surface-secondary text-xs font-medium text-content-secondary overflow-hidden md:min-w-[2.75rem]",
								"cursor-pointer border-0 transition-colors hover:bg-surface-tertiary hover:text-content-primary",
								"size-7 justify-center p-0 md:size-auto md:max-w-[200px] md:justify-start md:px-2 md:py-0.5",
							)}
						>
							<StatusIcon
								type={effectiveType}
								className="size-icon-sm shrink-0 md:size-3"
							/>
							<span className="hidden min-w-0 truncate md:inline">
								{workspace.name}
							</span>
							<ChevronDownIcon
								className={cn(
									"hidden size-3 shrink-0 opacity-60 transition-transform md:block",
									open && "rotate-180",
								)}
							/>
						</button>
					</DropdownMenuTrigger>
				</TooltipTrigger>
				<TooltipContent className="hidden md:block">
					{statusLabel}
				</TooltipContent>
			</Tooltip>

			<DropdownMenuContent
				side="top"
				align="start"
				className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-48 p-1 [&_[role=menuitem]]:text-xs [&_[role=menuitem]]:py-1 [&_svg]:!size-3.5 [&_img]:!size-3.5"
			>
				{showPortsView ? (
					<MobilePortsPanel
						workspace={workspace}
						agent={agent}
						host={host}
						isOpen={open}
						onBack={() => {
							setFocusPortsOnMain(true);
							setView("main");
						}}
					/>
				) : (
					<>
						{hasVSCode && (
							<VSCodeMenuItem
								variant="vscode"
								label="VS Code"
								workspace={workspace}
								agent={agent}
								chatId={chatId}
								folder={folder}
								isRunning={isRunning}
								generateKey={generateKey}
								isGeneratingKey={isGeneratingKey}
							/>
						)}
						{hasVSCodeInsiders && (
							<VSCodeMenuItem
								variant="vscode-insiders"
								label="VS Code Insiders"
								workspace={workspace}
								agent={agent}
								chatId={chatId}
								folder={folder}
								isRunning={isRunning}
								generateKey={generateKey}
								isGeneratingKey={isGeneratingKey}
							/>
						)}
						{userApps.map((app) => (
							<AppMenuItem
								key={app.id}
								app={app}
								workspace={workspace}
								agent={agent}
								isRunning={isRunning}
							/>
						))}
						{hasTerminal && (
							<TerminalMenuItem
								workspace={workspace}
								agent={agent}
								isRunning={isRunning}
							/>
						)}
						{portForwardingEnabled && (
							<PortsMenuItem
								workspace={workspace}
								agent={agent}
								host={host}
								isOpen={open}
								isRunning={isRunning}
								isBelowMd={isBelowMd}
								focusOnMount={focusPortsOnMain}
								onFocusApplied={() => setFocusPortsOnMain(false)}
								onSelectInline={() => {
									setFocusPortsOnMain(false);
									setView("ports");
								}}
							/>
						)}
						{hasItemsAboveSeparator && (
							<DropdownMenuSeparator className="my-1" />
						)}

						{sshCommand && <CopySSHMenuItem sshCommand={sshCommand} />}
						<DropdownMenuItem asChild>
							<Link to={route} target="_blank" rel="noreferrer">
								<MonitorIcon className="size-3.5" />
								View Workspace
							</Link>
						</DropdownMenuItem>
					</>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

// Reactive wrapper so callers re-render when the viewport crosses `md`.
const subscribeBelowMdViewport = (onStoreChange: () => void) => {
	const mediaQuery = matchMedia("(max-width: 767px)");
	mediaQuery.addEventListener("change", onStoreChange);
	return () => mediaQuery.removeEventListener("change", onStoreChange);
};

const useIsBelowMdViewport = (): boolean => {
	return useSyncExternalStore(subscribeBelowMdViewport, isBelowMdViewport);
};

interface PortsData {
	listeningPorts: readonly WorkspaceAgentListeningPort[] | undefined;
	sharedPorts: readonly WorkspaceAgentPortShare[] | undefined;
	privateListeningPorts: readonly WorkspaceAgentListeningPort[];
	totalCount: number | undefined;
	protocol: "http" | "https";
}

const usePortsData = (
	workspace: Workspace,
	agent: WorkspaceAgent,
	enabled: boolean,
): PortsData => {
	const protocol = getWorkspaceListeningPortsProtocol(workspace.id);

	const { data: listeningPorts } = useQuery({
		queryKey: ["portForward", agent.id],
		queryFn: () => API.getAgentListeningPorts(agent.id),
		enabled,
		refetchInterval: enabled ? 5_000 : false,
		staleTime: 0,
		select: (res) => res.ports,
	});

	const { data: sharedPorts } = useQuery({
		...workspacePortShares(workspace.id),
		enabled,
		staleTime: 0,
		select: (res) => res.shares.filter((s) => s.agent_name === agent.name),
	});

	// Listening ports that haven't been explicitly shared appear in their own
	// section; shared ports bubble up to the "Shared" section.
	const sharedPortNumbers = new Set((sharedPorts ?? []).map((s) => s.port));
	const privateListeningPorts = (listeningPorts ?? []).filter(
		(p) => !sharedPortNumbers.has(p.port),
	);

	const totalCount =
		listeningPorts !== undefined ? listeningPorts.length : undefined;

	return {
		listeningPorts,
		sharedPorts,
		privateListeningPorts,
		totalCount,
		protocol,
	};
};

const PortsMenuItem: FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	isOpen: boolean;
	isRunning: boolean;
	isBelowMd: boolean;
	focusOnMount: boolean;
	onFocusApplied: () => void;
	onSelectInline: () => void;
}> = ({
	workspace,
	agent,
	host,
	isOpen,
	isRunning,
	isBelowMd,
	focusOnMount,
	onFocusApplied,
	onSelectInline,
}) => {
	const isConnected = agent.status === "connected";
	const enabled = isOpen && isConnected;
	const itemRef = useRef<HTMLDivElement>(null);

	const portsData = usePortsData(workspace, agent, enabled);

	const label =
		portsData.totalCount !== undefined
			? `Ports (${portsData.totalCount})`
			: "Ports";

	useEffect(() => {
		if (!focusOnMount || !isBelowMd) {
			return;
		}
		itemRef.current?.focus();
		onFocusApplied();
	}, [focusOnMount, isBelowMd, onFocusApplied]);

	if (isBelowMd) {
		return (
			<DropdownMenuItem
				ref={itemRef}
				disabled={!isRunning}
				onSelect={(event) => {
					event.preventDefault();
					onSelectInline();
				}}
			>
				<NetworkIcon className="size-3.5" />
				{label}
				<ChevronRightIcon className="ml-auto size-3.5" />
			</DropdownMenuItem>
		);
	}

	return (
		<DropdownMenuSub>
			<DropdownMenuSubTrigger disabled={!isRunning}>
				<NetworkIcon className="size-3.5" />
				{label}
			</DropdownMenuSubTrigger>
			<DropdownMenuSubContent className="w-56 p-1 [&_[role=menuitem]]:text-xs [&_[role=menuitem]]:py-1 [&_svg]:!size-3.5">
				<PortsList
					host={host}
					agent={agent}
					workspace={workspace}
					data={portsData}
				/>
			</DropdownMenuSubContent>
		</DropdownMenuSub>
	);
};

const MobilePortsPanel: FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	isOpen: boolean;
	onBack: () => void;
}> = ({ workspace, agent, host, isOpen, onBack }) => {
	const isConnected = agent.status === "connected";
	const enabled = isOpen && isConnected;
	const backRef = useRef<HTMLDivElement>(null);
	const portsData = usePortsData(workspace, agent, enabled);

	useEffect(() => {
		backRef.current?.focus();
	}, []);

	return (
		<>
			<DropdownMenuItem
				ref={backRef}
				onSelect={(event) => {
					event.preventDefault();
					onBack();
				}}
			>
				<ArrowLeftIcon className="size-3.5" />
				Back
			</DropdownMenuItem>
			<DropdownMenuSeparator className="my-1" />
			<PortsList
				host={host}
				agent={agent}
				workspace={workspace}
				data={portsData}
			/>
		</>
	);
};

const PortsList: FC<{
	host: string;
	agent: WorkspaceAgent;
	workspace: Workspace;
	data: PortsData;
}> = ({ host, agent, workspace, data }) => {
	const route = `/@${workspace.owner_name}/${workspace.name}`;
	const { listeningPorts, sharedPorts, privateListeningPorts, protocol } = data;

	return (
		<>
			{privateListeningPorts.length > 0 && (
				<div className="px-2 pb-1.5 pt-1">
					<span className="text-xs font-semibold text-content-secondary">
						Listening Ports
					</span>
				</div>
			)}

			{privateListeningPorts.map((port) => (
				<ListeningPortItem
					key={port.port}
					port={port}
					host={host}
					agentName={agent.name}
					workspaceName={workspace.name}
					ownerName={workspace.owner_name}
					protocol={protocol}
				/>
			))}

			{listeningPorts !== undefined &&
				sharedPorts !== undefined &&
				privateListeningPorts.length === 0 &&
				sharedPorts.length === 0 && (
					<p className="px-2 py-2 text-center text-xs text-content-tertiary">
						No open ports detected.
					</p>
				)}

			{(sharedPorts ?? []).length > 0 && (
				<>
					<DropdownMenuSeparator className="my-1" />
					<div className="px-2 pb-1.5 pt-1">
						<span className="text-xs font-semibold text-content-secondary">
							Shared Ports
						</span>
					</div>
					{(sharedPorts ?? []).map((share) => (
						<SharedPortItem
							key={share.port}
							share={share}
							host={host}
							agentName={agent.name}
							workspaceName={workspace.name}
							ownerName={workspace.owner_name}
						/>
					))}
				</>
			)}

			<DropdownMenuSeparator className="my-1" />
			<DropdownMenuItem asChild>
				<Link to={route} target="_blank" rel="noreferrer">
					<ExternalLinkIcon className="size-3.5" />
					Manage sharing
				</Link>
			</DropdownMenuItem>
		</>
	);
};

const ListeningPortItem: FC<{
	port: WorkspaceAgentListeningPort;
	host: string;
	agentName: string;
	workspaceName: string;
	ownerName: string;
	protocol: "http" | "https";
}> = ({ port, host, agentName, workspaceName, ownerName, protocol }) => {
	const url = portForwardURL(
		host,
		port.port,
		agentName,
		workspaceName,
		ownerName,
		protocol,
	);
	return (
		<DropdownMenuItem asChild>
			<a href={url} target="_blank" rel="noreferrer">
				<RadioIcon className="size-3.5 shrink-0" />
				<span className="font-mono tabular-nums">{port.port}</span>
				{port.process_name !== "" && (
					<span className="truncate text-content-tertiary">
						{port.process_name}
					</span>
				)}
				<ExternalLinkIcon className="ml-auto size-3.5 shrink-0 opacity-50" />
			</a>
		</DropdownMenuItem>
	);
};

const SharedPortItem: FC<{
	share: WorkspaceAgentPortShare;
	host: string;
	agentName: string;
	workspaceName: string;
	ownerName: string;
}> = ({ share, host, agentName, workspaceName, ownerName }) => {
	const url = portForwardURL(
		host,
		share.port,
		agentName,
		workspaceName,
		ownerName,
		share.protocol,
	);
	const ShareIcon =
		share.share_level === "public"
			? LockOpenIcon
			: share.share_level === "organization"
				? BuildingIcon
				: LockIcon;
	return (
		<DropdownMenuItem asChild>
			<a href={url} target="_blank" rel="noreferrer">
				<ShareIcon className="size-3.5 shrink-0" />
				<span className="font-mono tabular-nums">{share.port}</span>
				<span className="truncate capitalize text-content-tertiary">
					{share.share_level}
				</span>
				<ExternalLinkIcon className="ml-auto size-3.5 shrink-0 opacity-50" />
			</a>
		</DropdownMenuItem>
	);
};

const VSCodeMenuItem: FC<{
	variant: "vscode" | "vscode-insiders";
	label: string;
	workspace: Workspace;
	agent: WorkspaceAgent;
	chatId: string;
	folder?: string;
	isRunning: boolean;
	generateKey: (
		variables: undefined,
		options: {
			onSuccess: (data: { key: string }) => void;
			onError: (error: unknown) => void;
		},
	) => void;
	isGeneratingKey: boolean;
}> = ({
	variant,
	label,
	workspace,
	agent,
	chatId,
	folder,
	isRunning,
	generateKey,
	isGeneratingKey,
}) => {
	const handleClick = () => {
		generateKey(undefined, {
			onSuccess: ({ key }) => {
				location.href = getVSCodeHref(variant, {
					owner: workspace.owner_name,
					workspace: workspace.name,
					token: key,
					agent: agent.name,
					folder: folder ?? agent.expanded_directory,
					chatId,
				});
			},
			onError: (error: unknown) => {
				toast.error(getErrorMessage(error, `Failed to open ${label}.`));
			},
		});
	};

	return (
		<DropdownMenuItem
			onSelect={handleClick}
			disabled={isGeneratingKey || !isRunning}
		>
			{variant === "vscode" ? (
				<VSCodeIcon className="size-3.5" />
			) : (
				<VSCodeInsidersIcon className="size-3.5" />
			)}
			{label}
		</DropdownMenuItem>
	);
};

const AppMenuItem: FC<{
	app: WorkspaceApp;
	workspace: Workspace;
	agent: WorkspaceAgent;
	isRunning: boolean;
}> = ({ app, workspace, agent, isRunning }) => {
	const link = useAppLink(app, { workspace, agent });

	const canClick =
		!isExternalApp(app) || !needsSessionToken(app) || link.hasToken;

	return (
		<DropdownMenuItem asChild disabled={!canClick || !isRunning}>
			<a
				href={canClick && isRunning ? link.href : undefined}
				onClick={link.onClick}
				target="_blank"
				rel="noreferrer"
			>
				{app.icon ? (
					<ExternalImage
						src={app.icon}
						alt=""
						className="size-3.5 rounded-sm"
					/>
				) : (
					<LayoutGridIcon className="size-3.5" />
				)}
				{link.label}
			</a>
		</DropdownMenuItem>
	);
};

const TerminalMenuItem: FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
	isRunning: boolean;
}> = ({ workspace, agent, isRunning }) => {
	const href = getTerminalHref({
		username: workspace.owner_name,
		workspace: workspace.name,
		agent: agent.name,
	});

	return (
		<DropdownMenuItem
			onSelect={() => {
				openAppInNewWindow(href);
			}}
			disabled={!isRunning}
		>
			<SquareTerminalIcon className="size-3.5" />
			Terminal
		</DropdownMenuItem>
	);
};

const CopySSHMenuItem: FC<{
	sshCommand: string;
}> = ({ sshCommand }) => {
	const { copyToClipboard } = useClipboard();

	return (
		<DropdownMenuItem
			onSelect={() => {
				void copyToClipboard(sshCommand);
			}}
		>
			<CopyIcon className="size-3.5" />
			Copy SSH Command
		</DropdownMenuItem>
	);
};
