import { useTheme } from "@emotion/react";
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutline";
import CircularProgress from "@mui/material/CircularProgress";
import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useProxy } from "contexts/ProxyContext";
import { type FC, useState } from "react";
import { createAppLinkHref } from "utils/apps";
import { generateRandomString } from "utils/random";
import { BaseIcon } from "./BaseIcon";
import { ShareIcon } from "./ShareIcon";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { Button } from "components/Button/Button";

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
}

export const AppLink: FC<AppLinkProps> = ({ app, workspace, agent }) => {
	const { proxy } = useProxy();
	const preferredPathBase = proxy.preferredPathAppURL;
	const appsHost = proxy.preferredWildcardHostname;
	const [fetchingSessionToken, setFetchingSessionToken] = useState(false);
	const [iconError, setIconError] = useState(false);

	const theme = useTheme();
	const username = workspace.owner_name;

	let appSlug = app.slug;
	let appDisplayName = app.display_name;
	if (!appSlug) {
		appSlug = appDisplayName;
	}
	if (!appDisplayName) {
		appDisplayName = appSlug;
	}

	const href = createAppLinkHref(
		window.location.protocol,
		preferredPathBase,
		appsHost,
		appSlug,
		username,
		workspace,
		agent,
		app,
	);

	// canClick is ONLY false when it's a subdomain app and the admin hasn't
	// enabled wildcard access URL or the session token is being fetched.
	//
	// To avoid bugs in the healthcheck code locking users out of apps, we no
	// longer block access to apps if they are unhealthy/initializing.
	let canClick = true;
	let icon = !iconError && (
		<BaseIcon app={app} onIconPathError={() => setIconError(true)} />
	);

	let primaryTooltip = "";
	if (app.health === "initializing") {
		icon = (
			// This is a hack to make the spinner appear in the center of the start
			// icon space
			<span
				css={{
					display: "flex",
					width: "100%",
					height: "100%",
					alignItems: "center",
					justifyContent: "center",
				}}
			>
				<CircularProgress size={14} />
			</span>
		);
		primaryTooltip = "Initializing...";
	}
	if (app.health === "unhealthy") {
		icon = <ErrorOutlineIcon css={{ color: theme.palette.warning.light }} />;
		primaryTooltip = "Unhealthy";
	}
	if (!appsHost && app.subdomain) {
		canClick = false;
		icon = <ErrorOutlineIcon css={{ color: theme.palette.grey[300] }} />;
		primaryTooltip =
			"Your admin has not configured subdomain application access";
	}
	if (fetchingSessionToken) {
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
		<Button disabled={!canClick} asChild>
			<a
				href={href}
				onClick={async (event) => {
					if (!canClick) {
						return;
					}

					event.preventDefault();

					// This is an external URI like "vscode://", so
					// it needs to be opened with the browser protocol handler.
					const shouldOpenAppExternally =
						app.external && !app.url.startsWith("http");

					if (shouldOpenAppExternally) {
						// This is a magic undocumented string that is replaced
						// with a brand-new session token from the backend.
						// This only exists for external URLs, and should only
						// be used internally, and is highly subject to break.
						const magicTokenString = "$SESSION_TOKEN";
						const hasMagicToken = href.indexOf(magicTokenString);
						let url = href;
						if (hasMagicToken !== -1) {
							setFetchingSessionToken(true);
							const key = await API.getApiKey();
							url = href.replaceAll(magicTokenString, key.key);
							setFetchingSessionToken(false);
						}

						// When browser recognizes the protocol and is able to navigate to the app,
						// it will blur away, and will stop the timer. Otherwise,
						// an error message will be displayed.
						const openAppExternallyFailedTimeout = 500;
						const openAppExternallyFailed = setTimeout(() => {
							displayError(
								`${app.display_name !== "" ? app.display_name : app.slug} must be installed first.`,
							);
						}, openAppExternallyFailedTimeout);
						window.addEventListener("blur", () => {
							clearTimeout(openAppExternallyFailed);
						});

						window.location.href = url;
						return;
					}

					switch (app.open_in) {
						case "slim-window": {
							window.open(
								href,
								Language.appTitle(appDisplayName, generateRandomString(12)),
								"width=900,height=600",
							);
							return;
						}
						default: {
							window.open(href);
							return;
						}
					}
				}}
			>
				{icon}
				{appDisplayName}
				{canShare && <ShareIcon app={app} />}
			</a>
		</Button>
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
