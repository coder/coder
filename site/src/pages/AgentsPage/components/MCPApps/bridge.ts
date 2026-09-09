import { asRecord } from "../ChatElements/runtimeTypeUtils";

interface HostContext {
	theme: "light" | "dark";
	displayMode: "inline" | "fullscreen";
	containerDimensions: { width: number; height: number };
	platform: "web";
	userAgent: string;
}

interface BridgeOptions {
	frame: HTMLIFrameElement;
	hostVersion: string;
	getContext: () => HostContext;
	getToolData: () => { args: unknown; result: unknown };
	onReady: () => void;
	onSizeChange: (size: { height?: number; width?: number }) => void;
}

interface MCPAppConnection {
	disconnect: () => void;
	notifyHostContextChanged: (context: Partial<HostContext>) => void;
}

/**
 * Supports MCP App initialization, tool notifications, sizing, display mode, and links.
 * App-initiated tools/call and other server requests are rejected with -32601.
 */
export function connectMCPApp({
	frame,
	hostVersion,
	getContext,
	getToolData,
	onReady,
	onSizeChange,
}: BridgeOptions): MCPAppConnection {
	let initializing = false;
	let ready = false;
	const post = (message: object) =>
		frame.contentWindow?.postMessage({ jsonrpc: "2.0", ...message }, "*");
	const onMessage = (event: MessageEvent<unknown>) => {
		// The sandbox has an opaque origin, so only the source window identifies it.
		if (!frame.contentWindow || event.source !== frame.contentWindow) return;
		const data = asRecord(event.data);
		if (data?.jsonrpc !== "2.0" || typeof data.method !== "string") return;
		const params = asRecord(data.params);
		const hasID = typeof data.id === "string" || typeof data.id === "number";
		if (!hasID) {
			if (
				data.method === "ui/notifications/initialized" &&
				initializing &&
				!ready
			) {
				ready = true;
				const tool = getToolData();
				post({
					method: "ui/notifications/tool-input",
					params: { arguments: tool.args ?? {} },
				});
				post({
					method: "ui/notifications/tool-result",
					params: asRecord(tool.result) ?? { content: [], isError: false },
				});
				onReady();
			} else if (data.method === "ui/notifications/size-change" && ready) {
				const size: { height?: number; width?: number } = {};
				for (const dimension of ["height", "width"] as const) {
					const value = params?.[dimension];
					if (
						typeof value === "number" &&
						Number.isFinite(value) &&
						value > 0
					) {
						size[dimension] = Math.min(1200, Math.max(100, value));
					}
				}
				if (size.height !== undefined || size.width !== undefined) {
					onSizeChange(size);
				}
			} else if (data.method === "notifications/message") {
				// biome-ignore lint/suspicious/noConsole: MCP logging notifications belong in the browser debug console.
				console.debug("MCP App:", params);
			}
			return;
		}
		const reply = (result: object) => post({ id: data.id, result });
		const reject = (code: number, message: string) =>
			post({ id: data.id, error: { code, message } });
		switch (data.method) {
			case "ui/initialize":
				initializing = true;
				reply({
					protocolVersion: "2026-01-26",
					hostInfo: { name: "coder", version: hostVersion },
					hostCapabilities: { openLinks: {} },
					hostContext: getContext(),
				});
				break;
			case "ui/request-display-mode":
				reply({ mode: getContext().displayMode });
				break;
			case "ui/open-link": {
				let url: URL;
				try {
					if (typeof params?.url !== "string") throw new Error("Missing URL");
					url = new URL(params.url);
				} catch {
					reject(-32602, "A valid HTTP or HTTPS URL is required.");
					break;
				}
				if (url.protocol !== "https:" && url.protocol !== "http:") {
					reject(-32602, "Only HTTP and HTTPS links are allowed.");
					break;
				}
				window.open(url.href, "_blank", "noopener,noreferrer");
				reply({});
				break;
			}
			default:
				reject(-32601, "Method not found");
		}
	};
	window.addEventListener("message", onMessage);
	return {
		disconnect: () => window.removeEventListener("message", onMessage),
		notifyHostContextChanged: (context) => {
			if (!ready) return;
			post({
				method: "ui/notifications/host-context-changed",
				params: context,
			});
		},
	};
}
