import { useTheme } from "@emotion/react";
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutline";
import type * as TypesGen from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useProxy } from "contexts/ProxyContext";
import {
	getAppHref,
	needsSessionToken,
	openAppInNewWindow,
} from "modules/apps/apps";
import { type FC, useState } from "react";
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

const Language = {
	appTitle: (appName: string, identifier: string): string =>
		`${appName} - ${identifier}`,
};

export interface AppLinkProps {
	workspace: TypesGen.Workspace;
	app: TypesGen.WorkspaceApp;
	agent: TypesGen.WorkspaceAgent;
	token?: string;
}

export const AppLink: FC<AppLinkProps> = ({ app, workspace, agent, token }) => {
	const { proxy } = useProxy();
	const host = proxy.preferredWildcardHostname;
	const [iconError, setIconError] = useState(false);
	const theme = useTheme();
	const displayName = app.display_name ?? app.slug;
	const href = getAppHref(app, {
		agent,
		workspace,
		token,
		path: proxy.preferredPathAppURL,
		host: proxy.preferredWildcardHostname,
	});

	// canClick is ONLY false when it's a subdomain app and the admin hasn't
	// enabled wildcard access URL or the session token is being fetched.
	//
	// To avoid bugs in the healthcheck code locking users out of apps, we no
	// longer block access to apps if they are unhealthy/initializing.
	let canClick = true;
	let primaryTooltip = "";
	let icon = !iconError && (
		<BaseIcon app={app} onIconPathError={() => setIconError(true)} />
	);

	if (app.health === "initializing") {
		icon = <Spinner loading />;
		primaryTooltip = "Initializing...";
	}

	if (app.health === "unhealthy") {
		icon = <ErrorOutlineIcon css={{ color: theme.palette.warning.light }} />;
		primaryTooltip = "Unhealthy";
	}

	if (!host && app.subdomain) {
		canClick = false;
		icon = <ErrorOutlineIcon css={{ color: theme.palette.grey[300] }} />;
		primaryTooltip =
			"Your admin has not configured subdomain application access";
	}

	if (!token && needsSessionToken(app)) {
		canClick = false;
	}

	if (
		agent.lifecycle_state === "starting" &&
		agent.startup_script_behavior === "blocking"
	) {
		canClick = false;
	}

	const canShare = app.sharing_level !== "owner";

	const button = (
		<AgentButton asChild>
			<a
				href={canClick ? href : undefined}
				onClick={(event) => {
					if (!canClick) {
						return;
					}

					if (app.external) {
						// When browser recognizes the protocol and is able to navigate to the app,
						// it will blur away, and will stop the timer. Otherwise,
						// an error message will be displayed.
						const openAppExternallyFailedTimeout = 500;
						const openAppExternallyFailed = setTimeout(() => {
							displayError(`${displayName} must be installed first.`);
						}, openAppExternallyFailedTimeout);
						window.addEventListener("blur", () => {
							clearTimeout(openAppExternallyFailed);
						});
					}

					switch (app.open_in) {
						case "slim-window": {
							event.preventDefault();
							openAppInNewWindow(href);
							return;
						}
					}
				}}
			>
				{icon}
				{displayName}
				{canShare && <ShareIcon app={app} />}
			</a>
		</AgentButton>
	);

	if (primaryTooltip) {
		return (
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>{button}</TooltipTrigger>
					<TooltipContent>{primaryTooltip}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	return button;
};
