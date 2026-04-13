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
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	getTerminalHref,
	getVSCodeHref,
	openAppInNewWindow,
} from "#/modules/apps/apps";
import { useAppLink } from "#/modules/apps/useAppLink";
import { cn } from "#/utils/cn";
import { getWorkspaceStatusDisplay } from "./WorkspaceStatusIndicator";

const menuItemCls =
	"flex w-full cursor-pointer items-center gap-2 rounded-md border-0 bg-transparent px-2 py-1.5 text-left text-xs text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary disabled:pointer-events-none disabled:opacity-50";

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

	const badgeCls =
		"inline-flex shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary";

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<Tooltip open={open ? false : undefined}>
				<TooltipTrigger asChild>
					<PopoverTrigger asChild>
						<button
							type="button"
							className={cn(
								badgeCls,
								"cursor-pointer border-0 transition-colors hover:bg-surface-tertiary hover:text-content-primary",
							)}
						>
							{statusIcon}
							{workspace.name}
							<ChevronDownIcon
								className={cn(
									"size-3 opacity-60 transition-transform rotate-180",
									open && "rotate-0",
								)}
							/>
						</button>
					</PopoverTrigger>
				</TooltipTrigger>
				<TooltipContent>{statusLabel}</TooltipContent>
			</Tooltip>
			<PopoverContent side="top" align="start" className="w-48 p-1">
				<div className="flex flex-col">
					{hasVSCode && (
						<VSCodeMenuItem
							variant="vscode"
							label="Open in VS Code"
							workspace={workspace}
							agent={agent}
							chatId={chatId}
							folder={folder}
							onDone={() => setOpen(false)}
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
							onDone={() => setOpen(false)}
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
						<TerminalMenuItem
							workspace={workspace}
							agent={agent}
							onDone={() => setOpen(false)}
						/>
					)}
					{sshCommand && (
						<CopySSHMenuItem
							sshCommand={sshCommand}
							onDone={() => setOpen(false)}
						/>
					)}
					<div className="my-1 h-px bg-border-default" />
					<Link
						to={route}
						target="_blank"
						rel="noreferrer"
						className={cn(menuItemCls, "no-underline")}
						onClick={() => setOpen(false)}
					>
						<MonitorIcon className="size-3.5" />
						View Workspace
					</Link>
				</div>
			</PopoverContent>
		</Popover>
	);
};

const VSCodeMenuItem: FC<{
	variant: "vscode" | "vscode-insiders";
	label: string;
	workspace: Workspace;
	agent: WorkspaceAgent;
	chatId: string;
	folder?: string;
	onDone: () => void;
}> = ({ variant, label, workspace, agent, chatId, folder, onDone }) => {
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
					folder: folder ?? agent.expanded_directory,
					chatId,
				});
				onDone();
			},
			onError: () => {
				toast.error(`Failed to open ${label}.`);
			},
		});
	};

	return (
		<button type="button" className={menuItemCls} onClick={handleClick}>
			<ExternalLinkIcon className="size-3.5" />
			{label}
		</button>
	);
};

const AppMenuItem: FC<{
	app: WorkspaceApp;
	workspace: Workspace;
	agent: WorkspaceAgent;
}> = ({ app, workspace, agent }) => {
	const link = useAppLink(app, { workspace, agent });

	return (
		<a
			href={link.href}
			onClick={link.onClick}
			target="_blank"
			rel="noreferrer"
			className={cn(menuItemCls, "no-underline")}
		>
			{app.icon ? (
				<ExternalImage src={app.icon} className="size-3.5 rounded-sm" />
			) : (
				<LayoutGridIcon className="size-3.5" />
			)}
			{link.label}
		</a>
	);
};

const TerminalMenuItem: FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
	onDone: () => void;
}> = ({ workspace, agent, onDone }) => {
	const href = getTerminalHref({
		username: workspace.owner_name,
		workspace: workspace.name,
		agent: agent.name,
	});

	return (
		<button
			type="button"
			className={menuItemCls}
			onClick={() => {
				openAppInNewWindow(href);
				onDone();
			}}
		>
			<SquareTerminalIcon className="size-3.5" />
			Open Terminal
		</button>
	);
};

const CopySSHMenuItem: FC<{
	sshCommand: string;
	onDone: () => void;
}> = ({ sshCommand, onDone }) => {
	const handleCopySSH = async () => {
		try {
			await navigator.clipboard.writeText(sshCommand);
			toast.success("SSH command copied to clipboard");
		} catch {
			toast.error("Failed to copy SSH command");
		}
		onDone();
	};

	return (
		<button
			type="button"
			className={menuItemCls}
			onClick={() => void handleCopySSH()}
		>
			<CopyIcon className="size-3.5" />
			Copy SSH Command
		</button>
	);
};
