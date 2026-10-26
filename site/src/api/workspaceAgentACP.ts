import {
	type ClientConnection,
	client,
	methods,
	ndJsonStream,
	PROTOCOL_VERSION,
	type SessionNotification,
} from "@agentclientprotocol/sdk";
import { createWebSocket } from "./api";

/** A byte-stream ACP connection. Closing it never closes the native session. */
export class WorkspaceAgentACP {
	private connection?: ClientConnection;
	private socket?: WebSocket;

	constructor(
		private readonly agentID: string,
		private readonly onUpdate: (notification: SessionNotification) => void,
		private readonly onPermission: (sessionID: string) => void,
		private readonly onClose: (error: Error) => void,
	) {}

	async connect() {
		this.close();
		const socket = createWebSocket(
			`/api/v2/workspaceagents/${encodeURIComponent(this.agentID)}/experimental/acp`,
		);
		socket.binaryType = "arraybuffer";
		this.socket = socket;
		let inputController: ReadableStreamDefaultController<Uint8Array>;
		let ended = false;
		const input = new ReadableStream<Uint8Array>(
			{
				start(controller) {
					inputController = controller;
				},
			},
			{ highWaterMark: 64 * 1024, size: (bytes) => bytes.byteLength },
		);
		let rejectOpen: (error: Error) => void = () => {};
		const opened = new Promise<void>((resolve, reject) => {
			rejectOpen = reject;
			socket.addEventListener("open", () => resolve(), { once: true });
		});
		const fail = (error: Error) => {
			if (ended) return;
			ended = true;
			rejectOpen(error);
			inputController.error(error);
			connection.close(error);
			if (this.socket === socket) this.onClose(error);
			socket.close();
		};
		socket.addEventListener("message", (event: MessageEvent<unknown>) => {
			if (ended) return;
			if (!(event.data instanceof ArrayBuffer)) {
				fail(new Error("Expected a binary ACP byte stream."));
				return;
			}
			// Bound queued input independently of the SDK's per-message limit.
			if (
				event.data.byteLength > 4 * 1024 * 1024 ||
				(inputController.desiredSize ?? 0) < -4 * 1024 * 1024
			) {
				fail(
					new Error(
						"ACP stream exceeded its buffer limit. Reconnect to reload retained history.",
					),
				);
				return;
			}
			inputController.enqueue(new Uint8Array(event.data));
		});
		socket.addEventListener("close", (event) => {
			fail(
				new Error(
					event.code === 1011
						? "Native session connection failed. Check workspace agent startup logs."
						: "ACP disconnected. The native session may still be running. Reconnect to reload retained history; prompts are never resent.",
				),
			);
		});
		socket.addEventListener("error", () => {
			// The close event carries the server's setup failure code.
			if (socket.readyState === WebSocket.CLOSED) {
				fail(
					new Error(
						"Could not connect to workspace ACP. Check workspace access and native agent setup.",
					),
				);
			}
		});
		const output = new WritableStream<Uint8Array>({
			async write(bytes) {
				await opened;
				if (socket.readyState !== WebSocket.OPEN)
					throw new Error("ACP disconnected.");
				if (socket.bufferedAmount > 1024 * 1024)
					throw new Error(
						"ACP output buffer is full. Reconnect before continuing.",
					);
				socket.send(new Uint8Array(bytes));
			},
		});
		const connection = client({ name: "coder" })
			.onNotification(methods.client.session.update, ({ params }) => {
				if (this.socket === socket) this.onUpdate(params);
			})
			.onRequest(methods.client.session.requestPermission, ({ params }) => {
				if (this.socket === socket) this.onPermission(params.sessionId);
				return { outcome: { outcome: "cancelled" } };
			})
			.connect(
				ndJsonStream(output, input, { maxMessageBytes: 4 * 1024 * 1024 }),
			);
		this.connection = connection;
		void connection.closed.then(() =>
			fail(
				new Error("ACP stream closed. Reconnect to reload retained history."),
			),
		);
		const timeout = setTimeout(
			() =>
				fail(
					new Error(
						"ACP connection timed out. Check workspace agent startup logs.",
					),
				),
			30_000,
		);
		try {
			await opened;
			const initialized = await connection.agent.request(
				methods.agent.initialize,
				{
					protocolVersion: PROTOCOL_VERSION,
					clientInfo: { name: "coder", version: "1" },
					clientCapabilities: {
						fs: { readTextFile: false, writeTextFile: false },
						terminal: false,
					},
				},
			);
			if (
				initialized.protocolVersion !== PROTOCOL_VERSION ||
				!initialized.agentCapabilities?.loadSession ||
				!initialized.agentCapabilities.sessionCapabilities?.list
			) {
				throw new Error(
					"The workspace ACP adapter must support session listing and loading.",
				);
			}
			return initialized;
		} catch (error) {
			fail(error instanceof Error ? error : new Error(String(error)));
			throw error;
		} finally {
			clearTimeout(timeout);
		}
	}

	get connected() {
		return this.connection !== undefined && !this.connection.signal.aborted;
	}

	get agent() {
		if (!this.connection || this.connection.signal.aborted)
			throw new Error("Connect to workspace ACP first.");
		return this.connection.agent;
	}

	close() {
		const socket = this.socket;
		this.socket = undefined;
		this.connection?.close();
		this.connection = undefined;
		socket?.close(1000);
	}
}
