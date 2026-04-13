import {
	ChevronDownIcon,
	CopyIcon,
	ExternalLinkIcon,
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
	needsSessionToken,
	openAppInNewWindow,
} from "#/modules/apps/apps";
import { useAppLink } from "#/modules/apps/useAppLink";
import { cn } from "#/utils/cn";
import { getWorkspaceStatusDisplay } from "./workspaceStatusDisplay";

const badgeCls =
	"inline-flex shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary";

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
	const route = `/@${workspace.owner_name}/${workspace.name}`;

	const { statusLabel, statusIcon } = getWorkspaceStatusDisplay(
		workspace,
		agent,
	);

	const builtinApps = new Set(agent.display_apps);
	const hasVSCode = builtinApps.has("vscode");
	const hasVSCodeInsiders = builtinApps.has("vscode_insiders");
	const hasTerminal = builtinApps.has("web_terminal");

	const userApps = agent.apps.filter((app) => !app.hidden);

	const hasItemsAboveSeparator =
		hasVSCode ||
		hasVSCodeInsiders ||
		userApps.length > 0 ||
		hasTerminal ||
		!!sshCommand;

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<Tooltip open={open ? false : undefined}>
				<TooltipTrigger asChild>
					<DropdownMenuTrigger asChild>
						<button
							type="button"
							aria-label={`${workspace.name} workspace menu`}
							className={cn(
								badgeCls,
								"cursor-pointer border-0 transition-colors hover:bg-surface-tertiary hover:text-content-primary",
							)}
						>
							{statusIcon}
							{workspace.name}
							{/* The menu opens upward (side="top"), so the chevron
							   points toward the menu when closed (default) and
							   away when open (rotate-180). */}
							<ChevronDownIcon
								className={cn(
									"size-3 opacity-60 transition-transform",
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
						label="Open in VS Code"
						workspace={workspace}
						agent={agent}
						chatId={chatId}
						folder={folder}
					/>
				)}
				{hasVSCodeInsiders && (
					<VSCodeMenuItem
						variant="vscode-insiders"
						label="Open in VS Code Insiders"
						workspace={workspace}
						agent={agent}
						chatId={chatId}
						folder={folder}
					/>
				)}
				{userApps.map((app) => (
					<AppMenuItem
						key={app.id}
						app={app}
						workspace={workspace}
						agent={agent}
					/>
				))}
				{hasTerminal && (
					<TerminalMenuItem workspace={workspace} agent={agent} />
				)}
				{sshCommand && <CopySSHMenuItem sshCommand={sshCommand} />}
				{hasItemsAboveSeparator && <DropdownMenuSeparator className="my-1" />}
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
}> = ({ variant, label, workspace, agent, chatId, folder }) => {
	const { mutate: generateKey, isPending } = useMutation({
		mutationFn: () => API.getApiKey(),
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
		onError: () => {
			toast.error(`Failed to open ${label}.`);
		},
	});

	const handleClick = () => {
		generateKey();
	};

	return (
		<DropdownMenuItem onSelect={handleClick} disabled={isPending}>
			<ExternalLinkIcon className="size-3.5" />
			{label}
		</DropdownMenuItem>
	);
};

const AppMenuItem: FC<{
	app: WorkspaceApp;
	workspace: Workspace;
	agent: WorkspaceAgent;
}> = ({ app, workspace, agent }) => {
	const link = useAppLink(app, { workspace, agent });

	const canClick =
		!isExternalApp(app) || !needsSessionToken(app) || link.hasToken;

	return (
		<DropdownMenuItem asChild disabled={!canClick}>
			<a
				href={canClick ? link.href : undefined}
				onClick={link.onClick}
				target="_blank"
				rel="noreferrer"
			>
				{app.icon ? (
					<ExternalImage src={app.icon} className="size-3.5 rounded-sm" />
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
			<SquareTerminalIcon className="size-3.5" />
			Open Terminal
		</DropdownMenuItem>
	);
};

const CopySSHMenuItem: FC<{
	sshCommand: string;
}> = ({ sshCommand }) => {
	const handleCopySSH = async () => {
		try {
			await navigator.clipboard.writeText(sshCommand);
			toast.success("SSH command copied to clipboard");
		} catch {
			toast.error("Failed to copy SSH command");
		}
	};

	return (
		<DropdownMenuItem onSelect={() => void handleCopySSH()}>
			<CopyIcon className="size-3.5" />
			Copy SSH Command
		</DropdownMenuItem>
	);
};
