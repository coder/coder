import { apiKey } from "api/queries/users";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useProxy } from "contexts/ProxyContext";
import type React from "react";
import { useQuery } from "react-query";
import {
	getAppHref,
	isExternalApp,
	needsSessionToken,
	openAppInNewWindow,
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
};

export const useAppLink = (
	app: WorkspaceApp,
	{ agent, workspace }: UseAppLinkParams,
): AppLink => {
	const label = app.display_name ?? app.slug;
	const { proxy } = useProxy();
	const { data: apiKeyResponse } = useQuery({
		...apiKey(),
		enabled: isExternalApp(app) && needsSessionToken(app),
	});

	const href = getAppHref(app, {
		agent,
		workspace,
		token: apiKeyResponse?.key,
		path: proxy.preferredPathAppURL,
		host: proxy.preferredWildcardHostname,
	});

	const onClick = (e: React.MouseEvent) => {
		if (!e.currentTarget.getAttribute("href")) {
			return;
		}

		if (app.external) {
			// When browser recognizes the protocol and is able to navigate to the app,
			// it will blur away, and will stop the timer. Otherwise,
			// an error message will be displayed.
			const openAppExternallyFailedTimeout = 500;
			const openAppExternallyFailed = setTimeout(() => {
				// Check if this is a JetBrains IDE app
				const isJetBrainsGateway = app.url?.startsWith("jetbrains-gateway:"); // starts with "jetbrains-gateway://connect#type=coder" (from https://registry.coder.com/modules/coder/jetbrains-gateway)
				const isJetBrainsToolbox = app.url?.startsWith("jetbrains:"); // starts with "jetbrains://gateway/coder" (from https://registry.coder.com/modules/coder/jetbrains)

				// Check if this is a coder:// URL
				const isCoderApp = app.url?.startsWith("coder:");

				if (isJetBrainsGateway) {
					displayError(
						`To use ${label}, you need to have JetBrains Gateway installed.`,
					);
				} else if (isJetBrainsToolbox) {
					displayError(
						`To use ${label}, you need to have JetBrains Toolbox installed.`,
					);
				} else if (isCoderApp) {
					displayError(
						`To use ${label} you need to have Coder Desktop installed`,
					);
				} else {
					displayError(`${label} must be installed first.`);
				}
			}, openAppExternallyFailedTimeout);
			window.addEventListener("blur", () => {
				clearTimeout(openAppExternallyFailed);
			});

			// External apps don't support open_in since they only should open
			// external apps.
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
		hasToken: !!apiKeyResponse?.key,
	};
};
