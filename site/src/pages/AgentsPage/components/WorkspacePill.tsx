import {
	ChevronDownIcon,
	CopyIcon,
	LayoutGridIcon,
	MonitorIcon,
	SquareTerminalIcon,
	UnlinkIcon,
} from "lucide-react";
import type { FC } from "react";
import { useEffect, useState } from "react";
import { useMutation } from "react-query";
import { Link } from "react-router";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
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
import { useIsBelowMdViewport } from "#/hooks/useIsBelowMdViewport";
import {
	getTerminalHref,
	getVSCodeHref,
	isExternalApp,
	needsSessionToken,
	openAppInNewWindow,
} from "#/modules/apps/apps";
import { useAppLink } from "#/modules/apps/useAppLink";
import {
	canShowPortForwarding,
	usePortsData,
} from "#/modules/resources/usePortsData";
import { cn } from "#/utils/cn";
import { getWorkspaceStatus, StatusIcon } from "./StatusIcon";
import { MobilePortsPanel, PortsMenuItem } from "./WorkspacePillPorts";

interface WorkspacePillProps {
	workspace: Workspace;
	agent: WorkspaceAgent;
	chatId: string;
	sshCommand?: string;
	folder?: string;
	onRemoveWorkspace?: () => void;
}

export const WorkspacePill: FC<WorkspacePillProps> = ({
	workspace,
	agent,
	chatId,
	sshCommand,
	folder,
	onRemoveWorkspace,
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
	const portForwardingEnabled = canShowPortForwarding(agent, host);

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

	const portsData = usePortsData(
		workspace,
		agent,
		open && agent.status === "connected" && portForwardingEnabled,
	);

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
			<span className="inline-flex min-w-0 items-center overflow-hidden rounded-full bg-surface-secondary text-xs font-medium text-content-secondary md:min-w-[2.75rem]">
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
									"inline-flex min-w-0 cursor-pointer items-center justify-center gap-1 rounded-full border-0 bg-transparent p-0 text-xs font-medium text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary",
									"size-7 md:size-auto md:max-w-[200px] md:justify-start md:px-2 md:py-0.5",
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
			</span>

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
						portsData={portsData}
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
								portsData={portsData}
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
						{onRemoveWorkspace && (
							<>
								<DropdownMenuSeparator className="my-1" />
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onClick={onRemoveWorkspace}
								>
									<UnlinkIcon className="size-3.5" />
									Detach workspace
								</DropdownMenuItem>
							</>
						)}
					</>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
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
