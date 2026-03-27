import { type FC, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Outlet, useBlocker, useParams, useSearchParams } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { useAuthContext } from "#/contexts/auth/AuthProvider";
import { ProxyProvider } from "#/contexts/ProxyContext";
import { DashboardProvider } from "#/modules/dashboard/DashboardProvider";
import { permissionChecks } from "#/modules/permissions";
import type { AgentsOutletContext } from "./AgentsPage";
import {
	bootstrapChatEmbedSession,
	EmbedContext,
} from "./components/EmbedContext";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
} from "./utils/usageLimitMessage";

type BootstrapMessage = {
	type: "coder:vscode-auth-bootstrap";
	payload: {
		token: string;
	};
};

const getBootstrapToken = (data: unknown): string | undefined => {
	if (typeof data !== "object" || data === null) {
		return undefined;
	}

	const message = data as Partial<BootstrapMessage>;
	if (message.type !== "coder:vscode-auth-bootstrap") {
		return undefined;
	}

	if (typeof message.payload !== "object" || message.payload === null) {
		return undefined;
	}

	const payload = message.payload as { token?: unknown };
	if (typeof payload.token !== "string") {
		return undefined;
	}

	const token = payload.token.trim();
	return token.length > 0 ? token : undefined;
};

const getThemeFromMessage = (data: unknown): "light" | "dark" | undefined => {
	if (typeof data !== "object" || data === null) {
		return undefined;
	}
	const msg = data as { type?: unknown; payload?: unknown };
	if (msg.type !== "coder:set-theme") {
		return undefined;
	}
	if (typeof msg.payload !== "object" || msg.payload === null) {
		return undefined;
	}
	const payload = msg.payload as { theme?: unknown };
	if (payload.theme !== "light" && payload.theme !== "dark") {
		return undefined;
	}
	return payload.theme;
};

/**
 * Sets the embed theme on <html> and marks it with a data
 * attribute so ThemeProvider skips its own class manipulation.
 * No-ops when the requested theme is already active.
 */
const applyEmbedTheme = (theme: "light" | "dark") => {
	const root = document.documentElement;
	if (root.dataset.embedTheme === theme) {
		return;
	}
	root.classList.remove("light", "dark");
	root.classList.add(theme);
	root.dataset.embedTheme = theme;
};

const AgentEmbedPage: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	if (!agentId) {
		throw new Error("AgentEmbedPage requires an agentId route parameter.");
	}

	const auth = useAuthContext();
	const queryClient = useQueryClient();
	const embedSessionMutation = useMutation(
		bootstrapChatEmbedSession({ checks: permissionChecks }, queryClient),
	);

	const inFlightBootstrapRef = useRef<Promise<unknown> | null>(null);

	const [chatErrorReasons, setChatErrorReasons] = useState<
		Record<string, ChatDetailError>
	>({});
	const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);

	const setChatErrorReason = (chatId: string, reason: ChatDetailError) => {
		const trimmedMessage = reason.message.trim();
		if (!chatId || !trimmedMessage) {
			return;
		}
		const nextReason: ChatDetailError = {
			...reason,
			message: trimmedMessage,
		};
		setChatErrorReasons((current) => {
			const existing = current[chatId];
			if (chatDetailErrorsEqual(existing, nextReason)) {
				return current;
			}
			return {
				...current,
				[chatId]: nextReason,
			};
		});
	};

	const clearChatErrorReason = (chatId: string) => {
		if (!chatId) {
			return;
		}
		setChatErrorReasons((current) => {
			if (!(chatId in current)) {
				return current;
			}
			const next = { ...current };
			delete next[chatId];
			return next;
		});
	};

	const requestArchiveAgent = (_chatId: string) => {};

	const requestUnarchiveAgent = (_chatId: string) => {};

	const requestArchiveAndDeleteWorkspace = (
		_chatId: string,
		_workspaceId: string,
	) => {};

	const onToggleSidebarCollapsed = () => {
		setIsSidebarCollapsed((current) => !current);
	};

	// Block navigations that leave the embed route and forward
	// the target URL to the parent frame.
	useBlocker(({ nextLocation }) => {
		if (nextLocation.pathname.startsWith(`/agents/${agentId}/embed`)) {
			return false;
		}
		window.parent.postMessage(
			{
				type: "coder:navigate",
				payload: {
					url: nextLocation.pathname + nextLocation.search + nextLocation.hash,
				},
			},
			"*",
		);
		return true;
	});

	// Apply the initial theme from the URL query param
	// (?theme=light|dark) or fall back to prefers-color-scheme.
	// useLayoutEffect runs before paint to prevent a flash.
	const [searchParams] = useSearchParams();
	useLayoutEffect(() => {
		const paramTheme = searchParams.get("theme");
		if (paramTheme === "light" || paramTheme === "dark") {
			applyEmbedTheme(paramTheme);
		} else {
			const prefersDark = window.matchMedia(
				"(prefers-color-scheme: dark)",
			).matches;
			applyEmbedTheme(prefersDark ? "dark" : "light");
		}
		return () => {
			document.documentElement.classList.remove("light", "dark");
			delete document.documentElement.dataset.embedTheme;
		};
	}, [searchParams]);

	// Shared ref for the chat scroll container. Passed through the
	// outlet context so AgentDetail attaches it to the DOM element
	// instead of creating its own.
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);

	// Listen for parent frame commands (e.g. theme changes).
	useEffect(() => {
		const parentWindow = window.parent;
		const handler = (event: MessageEvent) => {
			if (event.source !== parentWindow) {
				return;
			}
			const theme = getThemeFromMessage(event.data);
			if (theme) {
				applyEmbedTheme(theme);
			}
		};

		window.addEventListener("message", handler);
		return () => window.removeEventListener("message", handler);
	}, []);

	const onChatReady = () => {
		window.parent.postMessage({ type: "coder:chat-ready" }, "*");
	};

	const outletContext: AgentsOutletContext = {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		requestArchiveAgent,
		requestUnarchiveAgent,
		requestPinAgent: () => {},
		requestUnpinAgent: () => {},
		requestArchiveAndDeleteWorkspace,
		// Title regeneration is not supported in embed mode.
		isRegeneratingTitle: false,
		regeneratingTitleChatId: null,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		onExpandSidebar: () => {},
		onChatReady,
		scrollContainerRef,
	};

	// When signed out and not already bootstrapping, listen for the
	// postMessage from the parent frame carrying the session token.
	const isAwaitingBootstrapMessage =
		auth.isSignedOut &&
		!embedSessionMutation.isPending &&
		!embedSessionMutation.isError;

	useEffect(() => {
		if (!isAwaitingBootstrapMessage) {
			return;
		}

		const parentWindow = window.parent;

		const handleMessage = (event: MessageEvent) => {
			if (event.source !== parentWindow) {
				return;
			}

			const token = getBootstrapToken(event.data);
			if (!token || inFlightBootstrapRef.current) {
				return;
			}

			const bootstrapPromise = embedSessionMutation
				.mutateAsync(token)
				.catch(() => undefined)
				.finally(() => {
					inFlightBootstrapRef.current = null;
				});
			inFlightBootstrapRef.current = bootstrapPromise;
		};

		// Register the listener before notifying the parent so an
		// immediate bootstrap response is never missed.
		window.addEventListener("message", handleMessage);
		parentWindow.postMessage(
			{ type: "coder:vscode-ready", payload: { agentId } },
			"*",
		);
		return () => {
			window.removeEventListener("message", handleMessage);
		};
	}, [agentId, isAwaitingBootstrapMessage, embedSessionMutation]);

	const handleBootstrapRetry = () => {
		inFlightBootstrapRef.current = null;
		embedSessionMutation.reset();
	};

	if (auth.isSignedIn) {
		return (
			<EmbedContext value={{ isEmbedded: true }}>
				<DashboardProvider>
					<ProxyProvider>
						<Outlet context={outletContext} />
					</ProxyProvider>
				</DashboardProvider>
			</EmbedContext>
		);
	}

	if (embedSessionMutation.isError) {
		return (
			<div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-surface-primary px-6 text-center">
				<div className="space-y-2">
					<h1 className="text-xl font-semibold text-content-primary">
						Unable to start embedded agent.
					</h1>
					<p className="max-w-md text-sm text-content-secondary">
						{getErrorMessage(
							embedSessionMutation.error,
							"We couldn't exchange the VS Code bootstrap token for a session.",
						)}
					</p>
				</div>
				<Button onClick={handleBootstrapRetry}>Try again</Button>
			</div>
		);
	}

	if (embedSessionMutation.isPending) {
		return (
			<div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-surface-primary px-6 text-center">
				<Loader label="Signing in to embedded agent" />
				<p className="max-w-md text-sm text-content-secondary">
					Signing in to the embedded agent…
				</p>
			</div>
		);
	}

	// Either auth is loading or we're waiting for the bootstrap
	// postMessage from the parent frame.
	return (
		<div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-surface-primary px-6 text-center">
			<Loader label="Waiting for VS Code authentication" />
			<p className="max-w-md text-sm text-content-secondary">
				{auth.isLoading ? "Loading…" : "Waiting for VS Code authentication…"}
			</p>
		</div>
	);
};

export default AgentEmbedPage;
