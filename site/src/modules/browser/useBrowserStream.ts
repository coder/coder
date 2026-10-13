import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "#/components/Toaster/toast";
import {
	type BrowserClientMessage,
	BrowserFrameParser,
	type BrowserNavState,
	encodeClientMessage,
} from "./protocol";

export type BrowserStreamStatus = "connecting" | "connected" | "disconnected";

// Bounds latency: past this backlog the decoder skips to the next keyframe.
const MAX_DECODE_QUEUE = 8;
const RESIZE_DEBOUNCE_MS = 150;

/**
 * Connects to a workspace browser stream, decodes its video onto the
 * returned canvas and keeps the remote viewport sized to the canvas.
 */
export const useBrowserStream = (url: string | undefined) => {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const socketRef = useRef<WebSocket | null>(null);
	const [status, setStatus] = useState<BrowserStreamStatus>("connecting");
	const [navState, setNavState] = useState<BrowserNavState>();
	const [closeReason, setCloseReason] = useState<string>();
	const [attempt, setAttempt] = useState(0);

	const send = useCallback((msg: BrowserClientMessage) => {
		const socket = socketRef.current;
		if (socket?.readyState === WebSocket.OPEN) {
			socket.send(encodeClientMessage(msg));
		}
	}, []);

	const reconnect = useCallback(() => setAttempt((n) => n + 1), []);

	useEffect(() => {
		const canvas = canvasRef.current;
		if (!url || !canvas) {
			return;
		}
		const context = canvas.getContext("2d");
		setStatus("connecting");
		setCloseReason(undefined);

		const socket = new WebSocket(url);
		socket.binaryType = "arraybuffer";
		socketRef.current = socket;
		const parser = new BrowserFrameParser();
		let decoder: VideoDecoder | undefined;
		let codec: string | undefined;
		let waitingForKey = true;
		let timestamp = 0;

		const sendNow = (msg: BrowserClientMessage) => {
			if (socket.readyState === WebSocket.OPEN) {
				socket.send(encodeClientMessage(msg));
			}
		};
		const requestKeyframe = () => {
			waitingForKey = true;
			sendNow({ type: "keyframe" });
		};
		const createDecoder = () => {
			if (decoder && decoder.state !== "closed") {
				decoder.close();
			}
			if (!codec) {
				decoder = undefined;
				return;
			}
			const created = new VideoDecoder({
				output: (frame) => {
					context?.drawImage(frame, 0, 0, canvas.width, canvas.height);
					frame.close();
				},
				error: () => {
					if (decoder === created) {
						decoder = undefined;
					}
					requestKeyframe();
				},
			});
			created.configure({ codec, optimizeForLatency: true });
			decoder = created;
			waitingForKey = true;
		};

		const sendSize = () => {
			sendNow({
				type: "resize",
				width: Math.floor(canvas.clientWidth),
				height: Math.floor(canvas.clientHeight),
			});
		};
		let resizeTimer: ReturnType<typeof setTimeout> | undefined;
		const observer = new ResizeObserver(() => {
			clearTimeout(resizeTimer);
			resizeTimer = setTimeout(sendSize, RESIZE_DEBOUNCE_MS);
		});
		observer.observe(canvas);

		socket.onopen = () => {
			setStatus("connected");
			sendSize();
		};
		socket.onclose = (event) => {
			setStatus("disconnected");
			setCloseReason(event.reason || undefined);
		};
		socket.onmessage = (event) => {
			if (!(event.data instanceof ArrayBuffer)) {
				return;
			}
			for (const frame of parser.push(new Uint8Array(event.data))) {
				switch (frame.kind) {
					case "config":
						codec = frame.codec;
						canvas.width = frame.width;
						canvas.height = frame.height;
						createDecoder();
						break;
					case "video": {
						if (!decoder) {
							createDecoder();
						}
						if (!decoder || (waitingForKey && !frame.key)) {
							break;
						}
						if (!frame.key && decoder.decodeQueueSize > MAX_DECODE_QUEUE) {
							requestKeyframe();
							break;
						}
						waitingForKey = false;
						timestamp += 33_333;
						decoder.decode(
							new EncodedVideoChunk({
								type: frame.key ? "key" : "delta",
								timestamp,
								data: frame.data,
							}),
						);
						break;
					}
					case "state":
						setNavState(frame.state);
						break;
					case "error":
						toast.error(frame.message);
						break;
				}
			}
		};

		return () => {
			clearTimeout(resizeTimer);
			observer.disconnect();
			socket.onclose = null;
			socket.close();
			if (socketRef.current === socket) {
				socketRef.current = null;
			}
			if (decoder && decoder.state !== "closed") {
				decoder.close();
			}
		};
	}, [url, attempt]);

	return { canvasRef, status, navState, closeReason, send, reconnect };
};
