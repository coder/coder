import {
	Building2Icon,
	CircleAlertIcon,
	GlobeIcon,
	type LucideIcon,
	SquareArrowOutUpRightIcon,
	UsersIcon,
} from "lucide-react";
import { useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type { ButtonProps } from "#/components/Button/Button";
import { DropdownMenuItem } from "#/components/DropdownMenu/DropdownMenu";
import { Link } from "#/components/Link/Link";
import { Markdown } from "#/components/Markdown/Markdown";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useProxy } from "#/contexts/ProxyContext";
import {
	isAppBlockedByMissingWildcard,
	isAppUrlValid,
	SESSION_TOKEN_LINK_BLOCKED_MESSAGE,
} from "#/modules/apps/apps";
import { useAppLink } from "#/modules/apps/useAppLink";
import { docs } from "#/utils/docs";
import { AgentButton } from "../AgentButton";
import { BaseIcon } from "./BaseIcon";

export const DisplayAppNameMap: Record<TypesGen.DisplayApp, string> = {
	port_forwarding_helper: "Ports",
	ssh_helper: "SSH",
	vscode: "VS Code Desktop",
	vscode_insiders: "VS Code Insiders",
	web_terminal: "Terminal",
};

type AppLinkProps = {
	workspace: TypesGen.Workspace;
	app: TypesGen.WorkspaceApp;
	agent: TypesGen.WorkspaceAgent;
	grouped?: boolean;
	disabled?: boolean;
	size?: ButtonProps["size"];
};

export const AppLink: React.FC<AppLinkProps> = ({
	app,
	workspace,
	agent,
	grouped,
	disabled,
	size,
}) => {
	const { proxy } = useProxy();
	const [iconError, setIconError] = useState(false);
	const link = useAppLink(app, { agent, workspace });
	const status = getAppLinkStatus({
		app,
		agent,
		link,
		wildcardHostname: proxy.preferredWildcardHostname,
		disabled,
	});
	const icon =
		status.icon ??
		(!iconError && (
			<BaseIcon app={app} onIconPathError={() => setIconError(true)} />
		));

	const canShare = app.sharing_level !== "owner";
	const { shareTooltip, shareIcon: ShareIcon } = canShare
		? app.external
			? {
					shareTooltip: "Open external URL",
					shareIcon: SquareArrowOutUpRightIcon,
				}
			: shareDetails[app.sharing_level]
		: {
				shareTooltip: null,
				shareIcon: null,
			};

	// Token-minting external apps expose no navigable href (see useAppLink): the
	// URL is only complete after the on-click mint. Render them as a button so
	// they stay interactive. A bare anchor without href is styled and treated as
	// disabled by AgentButton, and middle-clicking one would otherwise launch
	// the custom protocol with an empty token.
	const opensViaClick = link.href === undefined;

	const content = (
		<>
			{icon}
			{link.label}
			{ShareIcon && <ShareIcon />}
		</>
	);

	const trigger = opensViaClick ? (
		// aria-disabled instead of disabled keeps the button focusable, so its
		// tooltip stays reachable from the keyboard.
		<button
			type="button"
			onClick={status.canClick ? link.onClick : undefined}
			aria-disabled={!status.canClick || undefined}
			className="aria-disabled:cursor-not-allowed"
		>
			{content}
		</button>
	) : (
		<a
			href={status.canClick ? link.href : undefined}
			onClick={link.onClick}
			target={app.open_in === "tab" ? "_blank" : undefined}
			rel={app.open_in === "tab" ? "noreferrer" : undefined}
		>
			{content}
		</a>
	);

	const button = grouped ? (
		<DropdownMenuItem
			asChild
			disabled={disabled}
			onSelect={status.canClick ? undefined : (e) => e.preventDefault()}
			className="w-full"
		>
			{trigger}
		</DropdownMenuItem>
	) : (
		<AgentButton asChild size={size}>
			{trigger}
		</AgentButton>
	);

	if (status.tooltip || app.tooltip) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>{button}</TooltipTrigger>
				<TooltipContent className="max-w-xs">
					{status.tooltip ||
						(app.tooltip && (
							<Markdown className="text-content-secondary prose-sm font-medium">
								{app.tooltip}
							</Markdown>
						))}
					{shareTooltip}
				</TooltipContent>
			</Tooltip>
		);
	}

	return button;
};

type AppLinkStatus = {
	canClick: boolean;
	// Replaces the app's own icon when set.
	icon?: React.ReactNode;
	tooltip?: React.ReactNode;
};

const getAppLinkStatus = ({
	app,
	agent,
	link,
	wildcardHostname,
	disabled,
}: {
	app: TypesGen.WorkspaceApp;
	agent: TypesGen.WorkspaceAgent;
	link: ReturnType<typeof useAppLink>;
	wildcardHostname: string;
	disabled?: boolean;
}): AppLinkStatus => {
	const hasValidUrl = isAppUrlValid(app);
	const isBlockedByStartup =
		agent.lifecycle_state === "starting" &&
		agent.startup_script_behavior === "blocking";
	// To avoid bugs in the healthcheck code locking users out of apps, app
	// health never blocks access.
	const canClick =
		!disabled &&
		!link.isLoading &&
		!link.isBlocked &&
		hasValidUrl &&
		!isAppBlockedByMissingWildcard(app, wildcardHostname) &&
		!isBlockedByStartup;

	const indicator = getAppLinkIndicator({
		app,
		agent,
		link,
		wildcardHostname,
		hasValidUrl,
	});
	return {
		canClick,
		icon: link.isLoading ? <Spinner loading /> : indicator.icon,
		tooltip: indicator.tooltip,
	};
};

// Returns the highest-priority problem to show for the app, if any.
const getAppLinkIndicator = ({
	app,
	agent,
	link,
	wildcardHostname,
	hasValidUrl,
}: {
	app: TypesGen.WorkspaceApp;
	agent: TypesGen.WorkspaceAgent;
	link: ReturnType<typeof useAppLink>;
	wildcardHostname: string;
	hasValidUrl: boolean;
}): Omit<AppLinkStatus, "canClick"> => {
	if (link.isBlocked || !hasValidUrl) {
		return {
			icon: <WarningIcon />,
			tooltip: (
				<>
					{hasValidUrl
						? SESSION_TOKEN_LINK_BLOCKED_MESSAGE
						: "This app has an invalid URL and can't be opened."}{" "}
					{agent.parent_id ? (
						<>
							Fix the app's <code>url</code> in the dev container's{" "}
							<code>devcontainer.json</code>.
						</>
					) : (
						<>
							Ask your template administrator to fix the app's <code>url</code>{" "}
							in the template's <code>coder_app</code> configuration.
						</>
					)}
				</>
			),
		};
	}

	if (app.subdomain_name && app.subdomain_name.length > 63) {
		return {
			icon: <WarningIcon />,
			tooltip: (
				<>
					Port forwarding will not work because hostname is too long, see the{" "}
					<Link
						href={docs(
							"/user-guides/workspace-access/port-forwarding#dashboard",
						)}
						target="_blank"
						size="sm"
					>
						documentation
					</Link>{" "}
					for more details
				</>
			),
		};
	}

	if (isAppBlockedByMissingWildcard(app, wildcardHostname)) {
		return {
			icon: (
				<CircleAlertIcon
					aria-hidden="true"
					className="size-icon-sm text-content-secondary"
				/>
			),
			tooltip: "Your admin has not configured subdomain application access",
		};
	}

	if (app.health === "unhealthy") {
		return { icon: <WarningIcon />, tooltip: "Unhealthy" };
	}

	if (app.health === "initializing") {
		return { icon: <Spinner loading />, tooltip: "Initializing..." };
	}

	return {};
};

const WarningIcon: React.FC = () => (
	<CircleAlertIcon
		aria-hidden="true"
		className="size-icon-sm text-content-warning"
	/>
);

const shareDetails: {
	[SharingLevel in TypesGen.WorkspaceAppSharingLevel as Exclude<
		SharingLevel,
		"owner"
	>]: { shareTooltip: string; shareIcon: LucideIcon };
} = {
	authenticated: {
		shareTooltip: "Shared with all authenticated users",
		shareIcon: UsersIcon,
	},
	organization: {
		shareTooltip: "Shared with organization members",
		shareIcon: Building2Icon,
	},
	public: {
		shareTooltip: "Shared publicly",
		shareIcon: GlobeIcon,
	},
};
