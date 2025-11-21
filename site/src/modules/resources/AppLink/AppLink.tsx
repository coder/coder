import type * as TypesGen from "api/typesGenerated";
import { DropdownMenuItem } from "components/DropdownMenu/DropdownMenu";
import { Link } from "components/Link/Link";
import { Markdown } from "components/Markdown/Markdown";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useProxy } from "contexts/ProxyContext";
import { CircleAlertIcon } from "lucide-react";
import { isExternalApp, needsSessionToken } from "modules/apps/apps";
import { useAppLink } from "modules/apps/useAppLink";
import { type FC, type ReactNode, useState } from "react";
import { AgentButton } from "../AgentButton";
import { BaseIcon } from "./BaseIcon";
import { ShareIcon } from "./ShareIcon";

export const DisplayAppNameMap: Record<TypesGen.DisplayApp, string> = {
	port_forwarding_helper: "Ports",
	ssh_helper: "SSH",
	vscode: "VS Code Desktop",
	vscode_insiders: "VS Code Insiders",
	web_terminal: "Terminal",
};

interface AppLinkProps {
	workspace: TypesGen.Workspace;
	app: TypesGen.WorkspaceApp;
	agent: TypesGen.WorkspaceAgent;
	grouped?: boolean;
}

export const AppLink: FC<AppLinkProps> = ({
	app,
	workspace,
	agent,
	grouped,
}) => {
	const { proxy } = useProxy();
	const host = proxy.preferredWildcardHostname;
	const [iconError, setIconError] = useState(false);
	const link = useAppLink(app, { agent, workspace });

	// canClick is ONLY false when it's a subdomain app and the admin hasn't
	// enabled wildcard access URL or the session token is being fetched.
	//
	// To avoid bugs in the healthcheck code locking users out of apps, we no
	// longer block access to apps if they are unhealthy/initializing.
	let canClick = true;
	let primaryTooltip: ReactNode = "";
	let icon = !iconError && (
		<BaseIcon app={app} onIconPathError={() => setIconError(true)} />
	);

	if (app.health === "initializing") {
		icon = <Spinner loading />;
		primaryTooltip = "Initializing...";
	}

	if (app.health === "unhealthy") {
		icon = (
			<CircleAlertIcon
				aria-hidden="true"
				className="size-icon-sm text-content-warning"
			/>
		);
		primaryTooltip = "Unhealthy";
	}

	if (!host && app.subdomain) {
		canClick = false;
		icon = (
			<CircleAlertIcon
				aria-hidden="true"
				className="size-icon-sm text-content-secondary"
			/>
		);
		primaryTooltip =
			"Your admin has not configured subdomain application access";
	}

	if (app.subdomain_name && app.subdomain_name.length > 63) {
		icon = (
			<CircleAlertIcon
				aria-hidden="true"
				className="size-icon-sm text-content-warning"
			/>
		);
		primaryTooltip = (
			<>
				Port forwarding will not work because hostname is too long, see the{" "}
				<Link
					href="https://coder.com/docs/user-guides/workspace-access/port-forwarding#dashboard"
					target="_blank"
					size="sm"
				>
					documentation
				</Link>{" "}
				for more details
			</>
		);
	}

	if (isExternalApp(app) && needsSessionToken(app) && !link.hasToken) {
		canClick = false;
	}

	if (
		agent.lifecycle_state === "starting" &&
		agent.startup_script_behavior === "blocking"
	) {
		canClick = false;
	}

	const canShare = app.sharing_level !== "owner";

	const button = grouped ? (
		<DropdownMenuItem asChild>
			<a href={canClick ? link.href : undefined} onClick={link.onClick}>
				{icon}
				{link.label}
				{canShare && <ShareIcon app={app} />}
			</a>
		</DropdownMenuItem>
	) : (
		<AgentButton asChild>
			<a href={canClick ? link.href : undefined} onClick={link.onClick}>
				{icon}
				{link.label}
				{canShare && <ShareIcon app={app} />}
			</a>
		</AgentButton>
	);

	if (primaryTooltip || app.tooltip) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>{button}</TooltipTrigger>
				<TooltipContent>
					{primaryTooltip ? (
						primaryTooltip
					) : app.tooltip ? (
						<Markdown className="text-content-secondary prose-sm font-medium wrap-anywhere">
							{app.tooltip}
						</Markdown>
					) : null}
				</TooltipContent>
			</Tooltip>
		);
	}

	return button;
};
