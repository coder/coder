import {
	ChevronDownIcon,
	CopyIcon,
	LayoutGridIcon,
	MonitorIcon,
	SquareTerminalIcon,
} from "lucide-react";
import type { FC } from "react";
import { useState } from "react";
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

	const builtinApps = new Set(agent.display_apps);
	const hasVSCode = builtinApps.has("vscode");
	const hasVSCodeInsiders = builtinApps.has("vscode_insiders");
	const hasTerminal = builtinApps.has("web_terminal");

	const userApps = agent.apps.filter((app) => !app.hidden);

	const hasItemsAboveSeparator =
		hasVSCode || hasVSCodeInsiders || userApps.length > 0 || hasTerminal;

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
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
								"inline-flex min-w-0 max-w-[200px] items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary",
								"cursor-pointer border-0 transition-colors hover:bg-surface-tertiary hover:text-content-primary",
							)}
						>
							<StatusIcon type={effectiveType} />
							<span className="truncate">{workspace.name}</span>
							{/* The menu opens upward (side="top"), so the chevron
							   points away from the menu when closed (default) and
							   toward it when open (rotate-180). */}
							<ChevronDownIcon
								className={cn(
									"size-3 shrink-0 opacity-60 transition-transform",
									open && "rotate-180",
								)}
							/>
						</button>
					</DropdownMenuTrigger>
				</TooltipTrigger>
				<TooltipContent>{statusLabel}</TooltipContent>
			</Tooltip>
			<DropdownMenuContent
				side="top"
				align="start"
				className="w-48 p-1 [&_[role=menuitem]]:text-xs [&_[role=menuitem]]:py-1 [&_svg]:!size-3.5 [&_img]:!size-3.5"
			>
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
				{hasItemsAboveSeparator && <DropdownMenuSeparator className="my-1" />}
				{sshCommand && <CopySSHMenuItem sshCommand={sshCommand} />}
				<DropdownMenuItem asChild>
					<Link to={route} target="_blank" rel="noreferrer">
						<MonitorIcon className="size-3.5" />
						View Workspace
					</Link>
				</DropdownMenuItem>
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
