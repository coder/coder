import {
	type AgentApp,
	type AgentConnection,
	ndJsonStream,
} from "@agentclientprotocol/sdk";

/** Runs the real ACP agent SDK behind a fragmented binary WebSocket test double. */
export function mockNativeAgentServer(app: AgentApp) {
	const sockets: MockSocket[] = [];
	class MockSocket extends EventTarget {
		static OPEN = 1;
		static CLOSED = 3;
		readyState = 0;
		binaryType = "blob";
		bufferedAmount = 0;
		readonly connection: AgentConnection;
		private input?: ReadableStreamDefaultController<Uint8Array>;
		constructor(readonly url: string) {
			super();
			sockets.push(this);
			const input = new ReadableStream<Uint8Array>({
				start: (controller) => {
					this.input = controller;
				},
			});
			const output = new WritableStream<Uint8Array>({
				write: (bytes) => {
					for (let offset = 0; offset < bytes.length; offset += 7) {
						this.dispatchEvent(
							new MessageEvent("message", {
								data: new Uint8Array(bytes.slice(offset, offset + 7)).buffer,
							}),
						);
					}
				},
			});
			this.connection = app.connect(ndJsonStream(output, input));
			queueMicrotask(() => {
				if (this.readyState === 3) return;
				this.readyState = 1;
				this.dispatchEvent(new Event("open"));
			});
		}
		send(bytes: Uint8Array) {
			this.input?.enqueue(bytes);
		}
		close(code = 1000) {
			if (this.readyState === 3) return;
			this.readyState = 3;
			this.connection.close();
			this.dispatchEvent(new CloseEvent("close", { code }));
		}
	}
	vi.stubGlobal("WebSocket", MockSocket);
	return {
		sockets,
		get socket() {
			const socket = sockets.at(-1);
			if (!socket) throw new Error("No ACP connection was opened.");
			return socket;
		},
	};
}
