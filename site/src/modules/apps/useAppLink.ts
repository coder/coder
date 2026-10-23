import type React from "react";
import { useQuery } from "react-query";
import { toast } from "sonner";
import { buildInfo } from "#/api/queries/buildInfo";
import { apiKey } from "#/api/queries/users";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import {
	getAppHref,
	isExternalApp,
	isSessionTokenLinkBlocked,
	needsSessionToken,
	openAppInNewWindow,
	SESSION_TOKEN_LINK_BLOCKED_MESSAGE,
} from "./apps";

type UseAppLinkParams = {
	workspace: Workspace;
	agent: WorkspaceAgent;
};

type AppLink = {
	href: string;
	onClick: (e: React.MouseEvent) => void;
	label: string;
	hasToken: boolean;
	isBlocked: boolean;
};

export const useAppLink = (
	app: WorkspaceApp,
	{ agent, workspace }: UseAppLinkParams,
): AppLink => {
	const label = app.display_name ?? app.slug;
	const { proxy } = useProxy();
	const { metadata } = useEmbeddedMetadata();
	const { data: build } = useQuery(buildInfo(metadata["build-info"]));
	const accessUrl = build?.dashboard_url;
	const isBlocked = isSessionTokenLinkBlocked(app, accessUrl);
	// Sub-agent apps never get a token, so they don't need a key to open.
	const isSubAgent = Boolean(agent.parent_id);
	const { data: apiKeyResponse } = useQuery({
		...apiKey(),
		enabled:
			isExternalApp(app) && needsSessionToken(app) && !isBlocked && !isSubAgent,
	});

	const href = getAppHref(app, {
		agent,
		workspace,
		token: apiKeyResponse?.key,
		accessUrl,
		path: proxy.preferredPathAppURL,
		host: proxy.preferredWildcardHostname,
	});

	const onClick = (e: React.MouseEvent) => {
		if (!e.currentTarget.getAttribute("href")) {
			return;
		}

		if (isBlocked) {
			e.preventDefault();
			toast.error(`Failed to open "${label}".`, {
				description: SESSION_TOKEN_LINK_BLOCKED_MESSAGE,
			});
			return;
		}

		// External apps with custom protocols (non-HTTP) need special handling
		// for error detection when the app isn't installed.
		const isExternalProtocolApp =
			app.external && app.url && !app.url.startsWith("http");

		if (isExternalProtocolApp) {
			// When browser recognizes the protocol and is able to navigate to the app,
			// it will blur away, and will stop the timer. Otherwise,
			// an error message will be displayed.
			const openAppExternallyFailedTimeout = 1500;
			const openAppExternallyFailed = setTimeout(() => {
				// Check if this is a JetBrains IDE app
				// starts with "jetbrains-gateway://connect#type=coder" (from https://registry.coder.com/modules/coder/jetbrains-gateway)
				const isJetBrainsGateway = app.url?.startsWith("jetbrains-gateway:");
				// starts with "jetbrains://gateway/coder" (from https://registry.coder.com/modules/coder/jetbrains)
				const isJetBrainsToolbox = app.url?.startsWith("jetbrains:");

				// Check if this is a coder:// URL
				const isCoderApp = app.url?.startsWith("coder:");

				if (isJetBrainsGateway) {
					toast.error(`Failed to open "${label}".`, {
						description: "JetBrains Gateway must be installed.",
					});
				} else if (isJetBrainsToolbox) {
					toast.error(`Failed to open "${label}".`, {
						description: "JetBrains Toolbox must be installed.",
					});
				} else if (isCoderApp) {
					toast.error(`Failed to open "${label}".`, {
						description: "Coder Desktop must be installed.",
					});
				} else {
					toast.error(`Failed to open "${label}".`, {
						description: "The app must be installed first.",
					});
				}
			}, openAppExternallyFailedTimeout);
			window.addEventListener("blur", () => {
				clearTimeout(openAppExternallyFailed);
			});

			// Custom protocol external apps don't support open_in since they
			// rely on the browser's protocol handling.
			return;
		}

		switch (app.open_in) {
			case "slim-window": {
				e.preventDefault();
				openAppInNewWindow(href);
				return;
			}
		}
	};

	return {
		href,
		onClick,
		label,
		hasToken: isSubAgent || Boolean(apiKeyResponse?.key),
		isBlocked,
	};
};
