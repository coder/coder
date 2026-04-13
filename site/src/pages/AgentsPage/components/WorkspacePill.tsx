import {
	ExternalLinkIcon,
	LayoutGridIcon,
	MonitorDotIcon,
	MonitorIcon,
	MonitorPauseIcon,
	MonitorXIcon,
	SquareTerminalIcon,
} from "lucide-react";
import type { FC } from "react";
import { useMutation } from "react-query";
import { Link } from "react-router";
import { toast } from "sonner";
import { API } from "#/api/api";
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
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	getTerminalHref,
	getVSCodeHref,
	isExternalApp,
	openAppInNewWindow,
} from "#/modules/apps/apps";
import { useAppLink } from "#/modules/apps/useAppLink";
import { cn } from "#/utils/cn";
import {
	type DisplayWorkspaceStatusType,
	getDisplayWorkspaceStatus,
} from "#/utils/workspace";

interface WorkspacePillProps {
	workspace: Workspace;
	agent: WorkspaceAgent;
	chatId: string;
	className?: string;
}

export const WorkspacePill: FC<WorkspacePillProps> = ({
	workspace,
	agent,
	chatId,
	className,
}) => {
	const route = `/@${workspace.owner_name}/${workspace.name}`;

	const { type, text } = getDisplayWorkspaceStatus(
		workspace.latest_build.status,
		workspace.latest_build.job,
	);
	const effectiveType = workspace.health.healthy ? type : "warning";
	const statusLabel = workspace.health.healthy
		? `Workspace ${text.toLowerCase()}`
		: `Workspace ${text.toLowerCase()} (unhealthy)`;
	const iconCls = "size-3";
	const statusIconMap: Record<DisplayWorkspaceStatusType, React.ReactNode> = {
		success: <MonitorIcon className={iconCls} />,
		active: <MonitorDotIcon className={iconCls} />,
		inactive: <MonitorPauseIcon className={iconCls} />,
		error: <MonitorXIcon className={iconCls} />,
		danger: <MonitorXIcon className={iconCls} />,
		warning: <MonitorXIcon className={iconCls} />,
	};
	const statusIcon = statusIconMap[effectiveType];

	const badgeCls = cn(
		"inline-flex shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary",
		className,
	);

	const builtinApps = new Set(agent.display_apps);
	const hasVSCode = builtinApps.has("vscode");
	const hasVSCodeInsiders = builtinApps.has("vscode_insiders");
	const hasTerminal = builtinApps.has("web_terminal");

	// User-configured external apps (non-hidden, non-web).
	const externalApps = agent.apps.filter(
		(app) => !app.hidden && isExternalApp(app),
	);

	const hasApps =
		hasVSCode || hasVSCodeInsiders || hasTerminal || externalApps.length > 0;

	if (!hasApps) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<Link
						to={route}
						target="_blank"
						rel="noreferrer"
						className={cn(
							badgeCls,
							"no-underline transition-colors hover:bg-surface-tertiary hover:text-content-primary",
						)}
					>
						{statusIcon}
						{workspace.name}
					</Link>
				</TooltipTrigger>
				<TooltipContent>{statusLabel}</TooltipContent>
			</Tooltip>
		);
	}

	return (
		<DropdownMenu>
			<Tooltip>
				<TooltipTrigger asChild>
					<DropdownMenuTrigger
						className={cn(
							badgeCls,
							"cursor-pointer border-0 transition-colors hover:bg-surface-tertiary hover:text-content-primary",
						)}
					>
						{statusIcon}
						{workspace.name}{" "}
					</DropdownMenuTrigger>
				</TooltipTrigger>
				<TooltipContent>{statusLabel}</TooltipContent>
			</Tooltip>
			<DropdownMenuContent
				align="start"
				className="[&_[role=menuitem]]:text-[13px]"
			>
				{hasVSCode && (
					<VSCodeMenuItem
						variant="vscode"
						label="Open in VS Code"
						workspace={workspace}
						agent={agent}
						chatId={chatId}
					/>
				)}
				{hasVSCodeInsiders && (
					<VSCodeMenuItem
						variant="vscode-insiders"
						label="Open in VS Code Insiders"
						workspace={workspace}
						agent={agent}
						chatId={chatId}
					/>
				)}
				{externalApps.map((app) => (
					<ExternalAppMenuItem
						key={app.id}
						app={app}
						workspace={workspace}
						agent={agent}
					/>
				))}
				{hasTerminal && (
					<TerminalMenuItem workspace={workspace} agent={agent} />
				)}
				<DropdownMenuSeparator />
				<DropdownMenuItem asChild>
					<Link to={route} target="_blank" rel="noreferrer">
						<ExternalLinkIcon className="!size-3.5" />
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
}> = ({ variant, label, workspace, agent, chatId }) => {
	const { mutate: generateKey, isPending } = useMutation({
		mutationFn: () => API.getApiKey(),
	});

	const handleClick = () => {
		if (isPending) return;
		generateKey(undefined, {
			onSuccess: ({ key }) => {
				location.href = getVSCodeHref(variant, {
					owner: workspace.owner_name,
					workspace: workspace.name,
					token: key,
					agent: agent.name,
					folder: agent.expanded_directory,
					chatId,
				});
			},
			onError: () => {
				toast.error(`Failed to open ${label}.`);
			},
		});
	};

	return (
		<DropdownMenuItem onSelect={handleClick}>
			<ExternalLinkIcon className="!size-3.5" />
			{label}
		</DropdownMenuItem>
	);
};

const ExternalAppMenuItem: FC<{
	app: WorkspaceApp;
	workspace: Workspace;
	agent: WorkspaceAgent;
}> = ({ app, workspace, agent }) => {
	const link = useAppLink(app, { workspace, agent });

	return (
		<DropdownMenuItem asChild>
			<a
				href={link.href}
				onClick={link.onClick}
				target="_blank"
				rel="noreferrer"
			>
				{app.icon ? (
					<ExternalImage src={app.icon} className="!size-3.5 rounded-sm" />
				) : (
					<LayoutGridIcon className="!size-3.5" />
				)}
				{link.label}
			</a>
		</DropdownMenuItem>
	);
};

const TerminalMenuItem: FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
}> = ({ workspace, agent }) => {
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
		>
			<SquareTerminalIcon className="!size-3.5" />
			Open Terminal
		</DropdownMenuItem>
	);
};
