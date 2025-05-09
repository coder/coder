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

export const useAppLink = (
	app: WorkspaceApp,
	{ agent, workspace }: UseAppLinkParams,
) => {
	const label = app.display_name ?? app.slug;
	const { proxy } = useProxy();
	const { data: apiKeyResponse } = useQuery({
		...apiKey(),
		enabled: isExternalApp(app) && needsSessionToken(app),
	});

	const href = getAppHref(app, {
		agent,
		workspace,
		token: apiKeyResponse?.key ?? "",
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
				displayError(`${label} must be installed first.`);
			}, openAppExternallyFailedTimeout);
			window.addEventListener("blur", () => {
				clearTimeout(openAppExternallyFailed);
			});
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
