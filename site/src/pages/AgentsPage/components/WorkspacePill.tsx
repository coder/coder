import {
	ExternalLinkIcon,
	LayoutGridIcon,
	SquareTerminalIcon,
} from "lucide-react";
import type { FC, ReactNode } from "react";
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
	appendChatIdToHref,
	getTerminalHref,
	getVSCodeHref,
	isExternalApp,
	openAppInNewWindow,
} from "#/modules/apps/apps";
import { useAppLink } from "#/modules/apps/useAppLink";
import { cn } from "#/utils/cn";

interface WorkspacePillProps {
	name: string;
	route: string;
	statusIcon: ReactNode;
	statusLabel: string;
	workspace: Workspace;
	agent: WorkspaceAgent;
	chatId: string;
	className?: string;
}

/**
 * Renders the workspace pill in the chat input area. When the
 * workspace has apps to show (external apps, built-in IDEs, and
 * terminal), the pill becomes a dropdown trigger. Otherwise it
 * falls back to a simple link to the workspace page.
 */
export const WorkspacePill: FC<WorkspacePillProps> = ({
	name,
	route,
	statusIcon,
	statusLabel,
	workspace,
	agent,
	chatId,
	className,
}) => {
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
						{name}
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
						{name}
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
						chatId={chatId}
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

/**
 * Renders a dropdown item for a built-in VS Code family display
 * app. Generates an API key on click (like the dashboard does)
 * and navigates to the protocol URL with chatId attached.
 */
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

/**
 * Renders a dropdown item for a user-configured external workspace
 * app. Uses the `useAppLink` hook for URL construction and token
 * handling, then post-processes the URL to inject chatId for
 * supported protocols.
 */
const ExternalAppMenuItem: FC<{
	app: WorkspaceApp;
	workspace: Workspace;
	agent: WorkspaceAgent;
	chatId: string;
}> = ({ app, workspace, agent, chatId }) => {
	const link = useAppLink(app, { workspace, agent });
	const href = appendChatIdToHref(link.href, chatId);

	return (
		<DropdownMenuItem asChild>
			<a href={href} onClick={link.onClick} target="_blank" rel="noreferrer">
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

/**
 * Renders a dropdown item for the built-in web terminal.
 */
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
