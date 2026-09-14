import type React from "react";
import { useMutation } from "react-query";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
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
	// Token-backed external apps intentionally expose no href: their URL is only
	// complete once a session token is minted on click.
	href: string | undefined;
	onClick: (e: React.MouseEvent) => void;
	label: string;
	isLoading: boolean;
};

export const useAppLink = (
	app: WorkspaceApp,
	{ agent, workspace }: UseAppLinkParams,
): AppLink => {
	const label = app.display_name ?? app.slug;
	const { proxy } = useProxy();

	// External apps that embed the session token in their URL need a freshly
	// minted key. We defer minting until the user clicks (see `onClick`) rather
	// than on mount, so that merely rendering a link no longer mints (and
	// audits) a session key for an app the user may never open.
	const requiresSessionToken = isExternalApp(app) && needsSessionToken(app);

	const buildHref = (token: string): string =>
		getAppHref(app, {
			agent,
			workspace,
			token,
			path: proxy.preferredPathAppURL,
			host: proxy.preferredWildcardHostname,
		});

	// Custom-protocol (non-HTTP) external apps can silently fail when the target
	// application isn't installed. The browser blurs when it hands control to
	// the protocol handler, which clears the timeout before the error fires.
	const notifyOnOpenExternalAppFailed = () => {
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
		window.addEventListener(
			"blur",
			() => {
				clearTimeout(openAppExternallyFailed);
			},
			{ once: true },
		);
	};

	// The success/error handlers live on the mutation (not on the `mutate` call)
	// so they still run when the triggering element unmounts before the request
	// settles, e.g. a dropdown menu item that closes on select. Callbacks passed
	// to `mutate` are dropped once the observer unmounts, which would otherwise
	// swallow both the navigation and the failure toast.
	const generateKeyMutation = useMutation({
		mutationFn: () => API.getApiKey(),
		onSuccess: ({ key }) => {
			notifyOnOpenExternalAppFailed();
			location.href = buildHref(key);
		},
		onError: (error) => {
			toast.error(getErrorMessage(error, `Failed to open "${label}".`));
		},
	});

	// Token-backed apps expose no navigable href: the token is minted on click
	// and the final URL is built then. Exposing a tokenless href would let
	// middle-click or "Open link" launch the custom protocol with an empty
	// token, so we omit it entirely. Non-token apps still render as anchors.
	const href = requiresSessionToken ? undefined : buildHref("");

	const onClick = (e: React.MouseEvent) => {
		// Apps that embed a session token mint it on click instead of on mount.
		// These are always custom-protocol (non-HTTP) external apps, so we build
		// the final URL with the freshly minted token and navigate to it via
		// `location.href`, relying on the browser's protocol handler.
		if (requiresSessionToken) {
			e.preventDefault();
			if (generateKeyMutation.isPending) {
				return;
			}
			generateKeyMutation.mutate();
			return;
		}

		if (!e.currentTarget.getAttribute("href")) {
			return;
		}

		// External apps with custom protocols (non-HTTP) need special handling
		// for error detection when the app isn't installed.
		const isExternalProtocolApp =
			app.external && app.url && !app.url.startsWith("http");

		if (isExternalProtocolApp) {
			notifyOnOpenExternalAppFailed();

			// Custom protocol external apps don't support open_in since they
			// rely on the browser's protocol handling.
			return;
		}

		switch (app.open_in) {
			case "slim-window": {
				e.preventDefault();
				if (href) {
					openAppInNewWindow(href);
				}
				return;
			}
		}
	};

	return {
		href,
		onClick,
		label,
		isLoading: generateKeyMutation.isPending,
	};
};
