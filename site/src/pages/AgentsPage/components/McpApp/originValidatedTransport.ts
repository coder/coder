import type { JSONRPCMessage, Transport } from "@modelcontextprotocol/client";
import { JSONRPCMessageSchema } from "@modelcontextprotocol/core";

/**
 * A `message` event is accepted only when it was posted by the expected
 * window from the expected origin. Both checks are required: the sandbox
 * origin hosts other chats' views, and the iframe can navigate itself
 * elsewhere.
 */
export const acceptsMessage = (
	event: Pick<MessageEvent, "source" | "origin">,
	source: Window | null,
	origin: string,
): boolean =>
	source !== null && event.source === source && event.origin === origin;

/**
 * postMessage transport for the MCP Apps host side that pins both the peer
 * window and its origin, and validates every inbound payload as JSON-RPC
 * before handing it to the protocol layer.
 */
export class OriginValidatedTransport implements Transport {
	onclose?: () => void;
	onerror?: (error: Error) => void;
	onmessage?: (message: JSONRPCMessage) => void;

	private listener: ((event: MessageEvent) => void) | undefined;

	constructor(
		private readonly targetWindow: Window,
		private readonly expectedOrigin: string,
	) {}

	async start(): Promise<void> {
		if (this.listener) {
			throw new Error("OriginValidatedTransport already started");
		}
		this.listener = (event: MessageEvent) => {
			if (!acceptsMessage(event, this.targetWindow, this.expectedOrigin)) {
				return;
			}
			const parsed = JSONRPCMessageSchema.safeParse(event.data);
			if (!parsed.success) {
				return;
			}
			this.onmessage?.(parsed.data);
		};
		window.addEventListener("message", this.listener);
	}

	async send(message: JSONRPCMessage): Promise<void> {
		this.targetWindow.postMessage(message, this.expectedOrigin);
	}

	async close(): Promise<void> {
		if (this.listener) {
			window.removeEventListener("message", this.listener);
			this.listener = undefined;
		}
		this.onclose?.();
	}
}
