import { Loader2 } from "lucide-react";
import {
	type FC,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import type { WorkspaceAgentPlugin } from "#/api/typesGenerated";
import { usePluginToken } from "../../hooks/usePluginToken";
import { isPluginMessage, type PluginContext } from "./pluginMessageBus";

const PLUGIN_READY_TIMEOUT_MS = 30_000;

interface PluginIframeProps {
	plugin: WorkspaceAgentPlugin;
	context: PluginContext;
	isVisible: boolean;
}

/**
 * Renders a plugin inside a sandboxed iframe. Manages the full
 * lifecycle: token minting → iframe load → init handshake →
 * ready acknowledgment → port URL requests → token refresh.
 *
 * The iframe is only mounted after the user activates the plugin
 * (isVisible=true for the first time). Once mounted, it stays
 * alive even when the tab is hidden.
 */
export const PluginIframe: FC<PluginIframeProps> = ({
	plugin,
	context,
	isVisible,
}) => {
	const iframeRef = useRef<HTMLIFrameElement>(null);
	const [isReady, setIsReady] = useState(false);
	const [hasActivated, setHasActivated] = useState(false);
	const [loadError, setLoadError] = useState<string | null>(null);
	const [hasTimedOut, setHasTimedOut] = useState(false);
	const [retryKey, setRetryKey] = useState(0);
	const initSentRef = useRef(false);
	const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	const { token, isLoading: isTokenLoading } = usePluginToken(
		plugin.slug,
		context.chatId,
		hasActivated,
	);

	// Track first activation so the iframe stays mounted after.
	useEffect(() => {
		if (isVisible && !hasActivated) {
			setHasActivated(true);
		}
	}, [isVisible, hasActivated]);

	// Send init message when both iframe is loaded and token is ready.
	const sendInit = useCallback(() => {
		const iframe = iframeRef.current;
		if (!iframe?.contentWindow || !token || initSentRef.current) {
			return;
		}
		iframe.contentWindow.postMessage(
			{
				type: "coder-plugin:init",
				payload: {
					apiUrl: context.apiUrl,
					pluginToken: token,
					workspaceId: context.workspaceId,
					agentId: context.agentId,
					chatId: context.chatId,
					pluginSlug: plugin.slug,
				},
			},
			"*",
		);
		initSentRef.current = true;
	}, [token, context, plugin.slug]);

	// Retry sending init when token becomes available after
	// iframe has already loaded (token mint is async).
	useEffect(() => {
		if (token && !initSentRef.current) {
			sendInit();
		}
	}, [token, sendInit]);

	// Start a timeout once init is sent. If we don't receive
	// coder-plugin:ready within 30s, surface an error so the
	// user isn't stuck on a spinner forever.
	useEffect(() => {
		if (!initSentRef.current || isReady) {
			return;
		}
		timeoutRef.current = setTimeout(() => {
			if (!isReady) {
				setHasTimedOut(true);
			}
		}, PLUGIN_READY_TIMEOUT_MS);
		return () => {
			if (timeoutRef.current) {
				clearTimeout(timeoutRef.current);
				timeoutRef.current = null;
			}
		};
		// retryKey resets the timeout after a retry.
	}, [initSentRef.current, isReady, retryKey]);

	// Listen for messages from the plugin iframe.
	useEffect(() => {
		const handler = (event: MessageEvent) => {
			if (!isPluginMessage(event.data)) {
				return;
			}

			switch (event.data.type) {
				case "coder-plugin:ready":
					setIsReady(true);
					break;
				case "coder-plugin:port-request": {
					// Resolve port URL and respond.
					const { port, requestId } = event.data.payload;
					const portUrl = `${context.apiUrl}/@me/${context.workspaceId}/apps/${port}`;
					iframeRef.current?.contentWindow?.postMessage(
						{
							type: "coder-plugin:port-response",
							payload: { port, url: portUrl, requestId },
						},
						"*",
					);
					break;
				}
			}
		};

		window.addEventListener("message", handler);
		return () => window.removeEventListener("message", handler);
	}, [context]);

	const handleIframeError = useCallback(() => {
		setLoadError(`Failed to load ${plugin.url}`);
	}, [plugin.url]);

	const handleRetry = useCallback(() => {
		setLoadError(null);
		setHasTimedOut(false);
		setIsReady(false);
		initSentRef.current = false;
		if (timeoutRef.current) {
			clearTimeout(timeoutRef.current);
			timeoutRef.current = null;
		}
		setRetryKey((k) => k + 1);
	}, []);

	// Send token refresh when token changes after init.
	useEffect(() => {
		if (!initSentRef.current || !token) {
			return;
		}
		iframeRef.current?.contentWindow?.postMessage(
			{
				type: "coder-plugin:token-refresh",
				payload: { pluginToken: token },
			},
			"*",
		);
	}, [token]);

	// Don't render anything until first activation.
	if (!hasActivated) {
		return (
			<div className="flex h-full items-center justify-center text-content-secondary">
				<p className="text-sm">
					Click to activate <strong>{plugin.display_name}</strong>
				</p>
			</div>
		);
	}

	const showError = loadError || hasTimedOut;

	if (showError) {
		return (
			<div className="flex h-full items-center justify-center text-content-secondary">
				<div className="flex flex-col items-center gap-3 text-center px-6">
					<p className="text-sm font-medium text-content-primary">
						Plugin failed to load
					</p>
					<p className="text-xs">
						{loadError ||
							"The plugin did not respond within 30 seconds."}
					</p>
					<p className="text-xs text-content-tertiary break-all">
						{plugin.url}
					</p>
					<button
						type="button"
						onClick={handleRetry}
						className="mt-2 rounded-md bg-surface-secondary px-3 py-1.5 text-xs font-medium hover:bg-surface-tertiary"
					>
						Retry
					</button>
				</div>
			</div>
		);
	}

	return (
		<div className="relative h-full w-full">
			{(!isReady || isTokenLoading) && (
				<div className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary">
					<div className="flex flex-col items-center gap-2 text-content-secondary">
						<Loader2 className="h-5 w-5 animate-spin" />
						<span className="text-xs">
							{isTokenLoading
								? "Minting plugin token..."
								: "Waiting for plugin..."}
						</span>
					</div>
				</div>
			)}
			<iframe
				key={retryKey}
				ref={iframeRef}
				src={plugin.url}
				sandbox="allow-scripts allow-forms allow-same-origin"
				title={`Plugin: ${plugin.display_name}`}
				className="h-full w-full border-0"
				onLoad={sendInit}
				onError={handleIframeError}
			/>
		</div>
	);
};
