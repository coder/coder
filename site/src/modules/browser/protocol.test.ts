import {
	BrowserFrameParser,
	encodeClientMessage,
	keyMessage,
	toVideoPoint,
} from "./protocol";

const frame = (type: number, payload: Uint8Array): Uint8Array => {
	const out = new Uint8Array(5 + payload.length);
	out[0] = type;
	new DataView(out.buffer).setUint32(1, payload.length);
	out.set(payload, 5);
	return out;
};

const json = (value: unknown) =>
	frame(2, new TextEncoder().encode(JSON.stringify(value)));

describe("BrowserFrameParser", () => {
	it("reassembles frames split across chunks", () => {
		const stream = new Uint8Array([
			...frame(1, new Uint8Array([1, 9, 8, 7])),
			...json({
				type: "state",
				url: "http://localhost:3000/",
				title: "App",
				loading: false,
				can_go_back: true,
				can_go_forward: false,
			}),
			...frame(1, new Uint8Array([0, 5])),
		]);
		const parser = new BrowserFrameParser();
		const frames = [];
		for (let i = 0; i < stream.length; i += 3) {
			frames.push(...parser.push(stream.slice(i, i + 3)));
		}
		expect(frames).toEqual([
			{ kind: "video", key: true, data: new Uint8Array([9, 8, 7]) },
			{
				kind: "state",
				state: {
					url: "http://localhost:3000/",
					title: "App",
					loading: false,
					canGoBack: true,
					canGoForward: false,
				},
			},
			{ kind: "video", key: false, data: new Uint8Array([5]) },
		]);
	});

	it("parses config and error messages", () => {
		const parser = new BrowserFrameParser();
		const frames = parser.push(
			new Uint8Array([
				...json({ type: "config", codec: "vp8", width: 800, height: 600 }),
				...json({ type: "error", message: "enter a URL" }),
			]),
		);
		expect(frames).toEqual([
			{ kind: "config", codec: "vp8", width: 800, height: 600 },
			{ kind: "error", message: "enter a URL" },
		]);
	});
});

describe("encodeClientMessage", () => {
	it("frames JSON messages", () => {
		const encoded = encodeClientMessage({ type: "reload" });
		expect(encoded[0]).toBe(2);
		const length = new DataView(encoded.buffer).getUint32(1);
		expect(
			JSON.parse(new TextDecoder().decode(encoded.subarray(5, 5 + length))),
		).toEqual({ type: "reload" });
	});
});

describe("keyMessage", () => {
	const base = {
		altKey: false,
		ctrlKey: false,
		metaKey: false,
		shiftKey: false,
	};

	it("sends text for printable keys", () => {
		expect(
			keyMessage({ ...base, key: "a", code: "KeyA", keyCode: 65 }, "down"),
		).toMatchObject({ text: "a", modifiers: 0 });
	});

	it("sends a carriage return for Enter", () => {
		expect(
			keyMessage({ ...base, key: "Enter", code: "Enter", keyCode: 13 }, "down"),
		).toMatchObject({ text: "\r" });
	});

	it("omits text for shortcuts and key releases", () => {
		expect(
			keyMessage(
				{ ...base, ctrlKey: true, key: "a", code: "KeyA", keyCode: 65 },
				"down",
			),
		).toMatchObject({ text: undefined, modifiers: 2 });
		expect(
			keyMessage({ ...base, key: "a", code: "KeyA", keyCode: 65 }, "up"),
		).toMatchObject({ text: undefined });
	});
});

describe("toVideoPoint", () => {
	it("accounts for letterboxing", () => {
		const rect = { left: 10, top: 20, width: 1000, height: 400 };
		// An 800x400 video in a 1000x400 box is centered with 100px bars.
		expect(toVideoPoint(110, 20, rect, 800, 400)).toEqual({ x: 0, y: 0 });
		expect(toVideoPoint(510, 220, rect, 800, 400)).toEqual({ x: 400, y: 200 });
	});
});
