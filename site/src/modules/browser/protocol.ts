/**
 * Wire protocol for the workspace browser stream. coderd relays the stream
 * as raw bytes, so WebSocket message boundaries carry no meaning: every
 * frame is [type u8][length u32 big-endian][payload].
 */

const FRAME_VIDEO = 1;
const FRAME_JSON = 2;
const HEADER_SIZE = 5;
const VIDEO_FLAG_KEY = 1;

export type BrowserNavState = {
	url: string;
	title: string;
	loading: boolean;
	canGoBack: boolean;
	canGoForward: boolean;
};

type BrowserFrame =
	| { kind: "video"; key: boolean; data: Uint8Array }
	| { kind: "config"; codec: string; width: number; height: number }
	| { kind: "state"; state: BrowserNavState }
	| { kind: "error"; message: string };

export type BrowserClientMessage =
	| { type: "resize"; width: number; height: number }
	| { type: "navigate"; url: string }
	| { type: "back" }
	| { type: "forward" }
	| { type: "reload" }
	| { type: "keyframe" }
	| { type: "insert_text"; text: string }
	| {
			type: "mouse";
			action: "down" | "up" | "move" | "wheel";
			x: number;
			y: number;
			button?: "left" | "middle" | "right" | "back" | "forward";
			buttons?: number;
			click_count?: number;
			delta_x?: number;
			delta_y?: number;
			modifiers?: number;
	  }
	| {
			type: "key";
			action: "down" | "up";
			key: string;
			code: string;
			key_code: number;
			text?: string;
			modifiers?: number;
	  };

const textDecoder = new TextDecoder();
const textEncoder = new TextEncoder();

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

const parseJSONFrame = (payload: Uint8Array): BrowserFrame | undefined => {
	let parsed: unknown;
	try {
		parsed = JSON.parse(textDecoder.decode(payload));
	} catch {
		return undefined;
	}
	if (!isRecord(parsed)) {
		return undefined;
	}
	switch (parsed.type) {
		case "config":
			return {
				kind: "config",
				codec: String(parsed.codec),
				width: Number(parsed.width),
				height: Number(parsed.height),
			};
		case "state":
			return {
				kind: "state",
				state: {
					url: String(parsed.url ?? ""),
					title: String(parsed.title ?? ""),
					loading: parsed.loading === true,
					canGoBack: parsed.can_go_back === true,
					canGoForward: parsed.can_go_forward === true,
				},
			};
		case "error":
			return { kind: "error", message: String(parsed.message ?? "") };
		default:
			return undefined;
	}
};

/** Reassembles frames from arbitrarily split stream chunks. */
export class BrowserFrameParser {
	private buffer: Uint8Array = new Uint8Array(0);

	push(chunk: Uint8Array): BrowserFrame[] {
		if (this.buffer.length === 0) {
			this.buffer = chunk;
		} else {
			const merged = new Uint8Array(this.buffer.length + chunk.length);
			merged.set(this.buffer);
			merged.set(chunk, this.buffer.length);
			this.buffer = merged;
		}

		const frames: BrowserFrame[] = [];
		let offset = 0;
		const view = new DataView(
			this.buffer.buffer,
			this.buffer.byteOffset,
			this.buffer.byteLength,
		);
		while (this.buffer.length - offset >= HEADER_SIZE) {
			const type = this.buffer[offset];
			const length = view.getUint32(offset + 1);
			const end = offset + HEADER_SIZE + length;
			if (end > this.buffer.length) {
				break;
			}
			const payload = this.buffer.subarray(offset + HEADER_SIZE, end);
			if (type === FRAME_VIDEO && payload.length > 1) {
				frames.push({
					kind: "video",
					key: (payload[0] & VIDEO_FLAG_KEY) !== 0,
					data: payload.subarray(1),
				});
			} else if (type === FRAME_JSON) {
				const frame = parseJSONFrame(payload);
				if (frame) {
					frames.push(frame);
				}
			}
			offset = end;
		}
		this.buffer = this.buffer.slice(offset);
		return frames;
	}
}

export const encodeClientMessage = (
	msg: BrowserClientMessage,
): Uint8Array<ArrayBuffer> => {
	const payload = textEncoder.encode(JSON.stringify(msg));
	const frame = new Uint8Array(HEADER_SIZE + payload.length);
	frame[0] = FRAME_JSON;
	new DataView(frame.buffer).setUint32(1, payload.length);
	frame.set(payload, HEADER_SIZE);
	return frame;
};

type ModifierState = {
	altKey: boolean;
	ctrlKey: boolean;
	metaKey: boolean;
	shiftKey: boolean;
};

/** Converts modifier keys to the CDP bit field. */
export const modifierBits = (e: ModifierState): number =>
	(e.altKey ? 1 : 0) |
	(e.ctrlKey ? 2 : 0) |
	(e.metaKey ? 4 : 0) |
	(e.shiftKey ? 8 : 0);

type KeyEventLike = ModifierState & {
	key: string;
	code: string;
	keyCode: number;
};

export const keyMessage = (
	e: KeyEventLike,
	action: "down" | "up",
): BrowserClientMessage => {
	let text: string | undefined;
	if (action === "down") {
		if (e.key === "Enter") {
			text = "\r";
		} else if ([...e.key].length === 1 && !e.ctrlKey && !e.metaKey) {
			text = e.key;
		}
	}
	return {
		type: "key",
		action,
		key: e.key,
		code: e.code,
		key_code: e.keyCode,
		text,
		modifiers: modifierBits(e),
	};
};

const mouseButtons = ["left", "middle", "right", "back", "forward"] as const;

export const mouseButton = (
	button: number,
): (typeof mouseButtons)[number] | undefined => mouseButtons[button];

/**
 * Maps a point in client coordinates to video pixels for a canvas drawn
 * with `object-fit: contain`.
 */
export const toVideoPoint = (
	clientX: number,
	clientY: number,
	rect: { left: number; top: number; width: number; height: number },
	videoWidth: number,
	videoHeight: number,
): { x: number; y: number } => {
	if (videoWidth <= 0 || videoHeight <= 0 || rect.width <= 0) {
		return { x: 0, y: 0 };
	}
	const scale = Math.min(rect.width / videoWidth, rect.height / videoHeight);
	const offsetX = (rect.width - videoWidth * scale) / 2;
	const offsetY = (rect.height - videoHeight * scale) / 2;
	const x = (clientX - rect.left - offsetX) / scale;
	const y = (clientY - rect.top - offsetY) / scale;
	return {
		x: Math.min(Math.max(x, 0), videoWidth - 1),
		y: Math.min(Math.max(y, 0), videoHeight - 1),
	};
};
