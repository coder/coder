import { asRecord } from "../ChatElements/runtimeTypeUtils";

interface HostContext {
	theme: "light" | "dark";
	displayMode: "inline" | "pip";
	containerDimensions: { width: number; height: number };
	platform: "web";
	userAgent: string;
}

const convertContent = (value: unknown): unknown => {
	const content = asRecord(value);
	if (!content) return value;
	const { media_type, resource, ...rest } = content;
	if (content.type === "resource") {
		const embedded = asRecord(resource);
		if (embedded) {
			const { mime_type, meta, ...body } = embedded;
			return {
				type: "resource",
				resource: {
					...body,
					...(mime_type ? { mimeType: mime_type } : {}),
					...(meta ? { _meta: meta } : {}),
				},
			};
		}
		return {
			type: "resource_link",
			uri: content.uri,
			name: content.uri,
			...(media_type ? { mimeType: media_type } : {}),
		};
	}
	return { ...rest, ...(media_type ? { mimeType: media_type } : {}) };
};

const toolResult = (value: unknown) => {
	const result = asRecord(value);
	return {
		content: Array.isArray(result?.content)
			? result.content.map(convertContent)
			: [],
		...(result?.structured_content !== undefined
			? { structuredContent: result.structured_content }
			: {}),
		...(typeof result?.is_error === "boolean"
			? { isError: result.is_error }
			: {}),
	};
};

interface BridgeOptions {
	frame: HTMLIFrameElement;
	hostVersion: string;
	getContext: () => HostContext;
	getToolData: () => { args: unknown; result: unknown };
	onReady: () => void;
	onSizeChange: (height: number) => void;
}

/** Connects one opaque-origin MCP App using the stable 2026-01-26 protocol. */
export function connectMCPApp({
	frame,
	hostVersion,
	getContext,
	getToolData,
	onReady,
	onSizeChange,
}: BridgeOptions): () => void {
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
					params: toolResult(tool.result),
				});
				onReady();
			} else if (data.method === "ui/notifications/size-change" && ready) {
				if (
					typeof params?.height === "number" &&
					Number.isFinite(params.height)
				) {
					onSizeChange(Math.min(1200, Math.max(100, params.height)));
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
	return () => window.removeEventListener("message", onMessage);
}
