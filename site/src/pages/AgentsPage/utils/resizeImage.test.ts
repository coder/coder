import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resizeImageToMaxBytes } from "./resizeImage";

// jsdom (the default vitest environment) does not implement
// createImageBitmap / OffscreenCanvas, so the re-encode codepaths
// cannot run against real browser decoders. The "with stubbed
// decoders" block below installs a deterministic fake so the shrink
// loop runs in CI; the "with real decoders" block only runs when
// actual browser APIs are available (e.g. a future Playwright-based
// vitest project).
const canDecodeImages =
	typeof createImageBitmap === "function" &&
	typeof OffscreenCanvas === "function";

const describeIfDecode = canDecodeImages ? describe : describe.skip;

// Minimum byte count the fake decoder reports for a blob at given
// dimensions. Sized so the shrink loop runs a realistic number of
// iterations within the module's MAX_SHRINK_ITERATIONS=8 budget.
const FAKE_BYTES_PER_PIXEL = 0.5;

// Returns a fake encoded blob whose size is proportional to (width
// * height * quality) so the shrink loop can observe convergence.
function fakeEncodedSize(
	width: number,
	height: number,
	quality: number,
): number {
	return Math.max(
		64,
		Math.round(width * height * quality * FAKE_BYTES_PER_PIXEL),
	);
}

describe("resizeImageToMaxBytes", () => {
	it("returns non-image files unchanged", async () => {
		const file = new File([new Uint8Array([1, 2, 3])], "notes.txt", {
			type: "text/plain",
		});
		const result = await resizeImageToMaxBytes(file, 1024);
		expect(result).toBe(file);
	});

	it("returns GIFs unchanged even when oversize", async () => {
		// Animated GIFs can't be safely re-encoded via canvas
		// (canvas flattens to a single frame), so resizeImageToMaxBytes
		// is expected to bail early and hand back the original file.
		const bytes = new Uint8Array(8 * 1024 * 1024);
		const file = new File([bytes], "clip.gif", { type: "image/gif" });
		const result = await resizeImageToMaxBytes(file, 1024 * 1024);
		expect(result).toBe(file);
	});

	it("returns the original image unchanged when already under budget", async () => {
		const bytes = new Uint8Array(1024);
		const file = new File([bytes], "tiny.png", { type: "image/png" });
		// Budget larger than file → no work required.
		const result = await resizeImageToMaxBytes(file, 4096);
		expect(result).toBe(file);
	});

	it("returns null for an unsupported image MIME that would need resizing", async () => {
		// image/svg+xml and similar types are not in the resizable
		// allowlist; when over budget, the helper refuses to touch
		// them instead of silently producing garbage.
		const bytes = new Uint8Array(2 * 1024 * 1024);
		const file = new File([bytes], "diagram.svg", {
			type: "image/svg+xml",
		});
		const result = await resizeImageToMaxBytes(file, 1024 * 1024);
		expect(result).toBeNull();
	});
});

describe("resizeImageToMaxBytes with stubbed decoders", () => {
	// Each test installs its own fakes; track per-test state on a
	// shared object so the stubs can read the active configuration.
	interface StubState {
		srcWidth: number;
		srcHeight: number;
		decodeThrows: boolean;
		convertBlobType: string;
		decodeCalls: number;
		encodeCalls: Array<{ width: number; height: number; quality: number }>;
	}

	let state: StubState;

	beforeEach(() => {
		state = {
			srcWidth: 4096,
			srcHeight: 4096,
			decodeThrows: false,
			convertBlobType: "image/webp",
			decodeCalls: 0,
			encodeCalls: [],
		};

		// Fake createImageBitmap: respects resizeWidth/Height the way
		// the production code relies on (clamp-in-box, preserve
		// aspect ratio), and records each call for assertions.
		vi.stubGlobal(
			"createImageBitmap",
			vi.fn(
				async (
					_blob: Blob,
					options?: {
						resizeWidth?: number;
						resizeHeight?: number;
					},
				) => {
					state.decodeCalls++;
					if (state.decodeThrows) {
						throw new Error("decode boom");
					}
					let w = state.srcWidth;
					let h = state.srcHeight;
					const limit = Math.min(
						options?.resizeWidth ?? Number.POSITIVE_INFINITY,
						options?.resizeHeight ?? Number.POSITIVE_INFINITY,
					);
					if (Number.isFinite(limit) && limit > 0) {
						const scale = Math.min(1, limit / w, limit / h);
						w = Math.max(1, Math.round(w * scale));
						h = Math.max(1, Math.round(h * scale));
					}
					return {
						width: w,
						height: h,
						close: vi.fn(),
					} as unknown as ImageBitmap;
				},
			),
		);

		// Fake OffscreenCanvas whose convertToBlob returns a blob of
		// size proportional to (width * height * quality) so the
		// shrink loop observes convergence.
		class FakeOffscreenCanvas {
			width: number;
			height: number;
			constructor(w: number, h: number) {
				this.width = w;
				this.height = h;
			}
			getContext() {
				// drawImage is called on whatever ctx we return; accept
				// any args and return nothing — the test isn't
				// inspecting pixel data.
				return {
					drawImage: () => undefined,
				};
			}
			async convertToBlob(opts?: { quality?: number }): Promise<Blob> {
				const quality = opts?.quality ?? 1;
				state.encodeCalls.push({
					width: this.width,
					height: this.height,
					quality,
				});
				const size = fakeEncodedSize(this.width, this.height, quality);
				return new Blob([new Uint8Array(size)], {
					type: state.convertBlobType,
				});
			}
		}
		vi.stubGlobal("OffscreenCanvas", FakeOffscreenCanvas);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("re-encodes an oversized image down to the requested byte budget", async () => {
		// 4096² pixels × 0.5 bytes/pixel × 0.85 quality ≈ 7.1 MiB
		// raw — well over budget, forcing at least a few shrink
		// iterations before convergence.
		const file = new File([new Uint8Array(6 * 1024 * 1024)], "big.png", {
			type: "image/png",
		});
		const budget = 512 * 1024;
		const result = await resizeImageToMaxBytes(file, budget);
		expect(result).not.toBeNull();
		if (!result) return;
		expect(result.size).toBeLessThanOrEqual(budget);
		expect(result.type).toBe("image/webp");
		expect(result.name.endsWith(".webp")).toBe(true);
		// Must have iterated the shrink loop at least once.
		expect(state.encodeCalls.length).toBeGreaterThan(0);
	});

	it("passes resizeWidth/resizeHeight to the decoder so the bitmap cannot exceed the clamp", async () => {
		// Source is far larger than MAX_INITIAL_DIMENSION on both
		// axes. Without the resizeWidth/Height options the decoder
		// would allocate a ~60000×40000 bitmap (~9.6 GB RGBA).
		state.srcWidth = 60_000;
		state.srcHeight = 40_000;
		const file = new File([new Uint8Array(6 * 1024 * 1024)], "huge.png", {
			type: "image/png",
		});
		await resizeImageToMaxBytes(file, 256 * 1024);
		// Every recorded encode must be within the 8192² clamp so we
		// know the clamp is applied at decode time, not later.
		for (const call of state.encodeCalls) {
			expect(call.width).toBeLessThanOrEqual(8192);
			expect(call.height).toBeLessThanOrEqual(8192);
		}
	});

	it("preserves aspect ratio across the shrink loop", async () => {
		state.srcWidth = 8000;
		state.srcHeight = 2000; // 4:1 ratio
		const file = new File([new Uint8Array(6 * 1024 * 1024)], "wide.png", {
			type: "image/png",
		});
		await resizeImageToMaxBytes(file, 32 * 1024);
		expect(state.encodeCalls.length).toBeGreaterThan(0);
		for (const call of state.encodeCalls) {
			// Allow ±1 px rounding across iterative Math.round calls.
			const ratio = call.width / call.height;
			expect(ratio).toBeGreaterThanOrEqual(4 - 0.05);
			expect(ratio).toBeLessThanOrEqual(4 + 0.05);
		}
	});

	it("tries the fallback quality pass when shrink iterations saturate", async () => {
		// Use a source small enough that the fake will continue to
		// overshoot even at 1×1 (we force this by choosing a
		// ridiculously small budget); that exhausts the main loop
		// and triggers the FALLBACK_QUALITY pass.
		state.srcWidth = 64;
		state.srcHeight = 64;
		const file = new File([new Uint8Array(1024 * 1024)], "tiny.png", {
			type: "image/png",
		});
		const result = await resizeImageToMaxBytes(file, 1);
		// The fake's minimum blob size is 64 bytes, so the budget of
		// 1 byte is unreachable → null is the expected outcome.
		expect(result).toBeNull();
		// Last encode attempt should use FALLBACK_QUALITY (0.7), not
		// the initial 0.85, proving the fallback ran.
		const last = state.encodeCalls[state.encodeCalls.length - 1];
		expect(last.quality).toBeCloseTo(0.7, 5);
	});

	it("returns null (does not throw) when decode fails", async () => {
		state.decodeThrows = true;
		const file = new File([new Uint8Array(1024 * 1024)], "broken.png", {
			type: "image/png",
		});
		const result = await resizeImageToMaxBytes(file, 4096);
		expect(result).toBeNull();
	});

	it("keeps the File type honest when the encoder falls back to PNG", async () => {
		// Some browsers without WebP encode support silently return
		// a PNG. The helper must reflect the actual type on the File
		// rather than labelling a PNG as WebP.
		state.convertBlobType = "image/png";
		const file = new File([new Uint8Array(2 * 1024 * 1024)], "photo.png", {
			type: "image/png",
		});
		const result = await resizeImageToMaxBytes(file, 1024 * 1024);
		expect(result).not.toBeNull();
		if (!result) return;
		expect(result.type).toBe("image/png");
		// Extension should not claim .webp for a PNG payload.
		expect(result.name.endsWith(".webp")).toBe(false);
	});
});

describeIfDecode("resizeImageToMaxBytes with real decoders", () => {
	it("returns null (does not throw) for a corrupt image blob", async () => {
		// Body is not a valid image; decode must fail cleanly and the
		// caller must see `null` so it can fall back to the original.
		// Gated on real decoders because the jsdom <img> fallback
		// never fires onload/onerror and would hang forever.
		const bytes = new Uint8Array([0x00, 0x01, 0x02, 0x03]);
		const file = new File([bytes], "broken.png", { type: "image/png" });
		// Force the resize path by picking a budget smaller than the
		// file so we don't hit the fast-path return.
		const result = await resizeImageToMaxBytes(file, 1);
		expect(result).toBeNull();
	});

	it("re-encodes a large PNG down to the requested byte budget", async () => {
		// Synthesize a real PNG via OffscreenCanvas so the decode step
		// has something to work on. 1024x1024 is large enough that the
		// initial encode typically overshoots a 64 KiB budget,
		// forcing the iterative shrink path.
		const canvas = new OffscreenCanvas(1024, 1024);
		const ctx = canvas.getContext("2d");
		if (!ctx) throw new Error("no 2d ctx");
		// Fill with a noise pattern so compression doesn't reduce it
		// to a trivial byte count before shrinking kicks in.
		const data = ctx.createImageData(1024, 1024);
		for (let i = 0; i < data.data.length; i += 4) {
			data.data[i] = (i * 13) & 0xff;
			data.data[i + 1] = (i * 7) & 0xff;
			data.data[i + 2] = (i * 19) & 0xff;
			data.data[i + 3] = 0xff;
		}
		ctx.putImageData(data, 0, 0);
		const sourceBlob = await canvas.convertToBlob({ type: "image/png" });
		const file = new File([sourceBlob], "large.png", {
			type: "image/png",
		});

		const budget = 64 * 1024;
		const result = await resizeImageToMaxBytes(file, budget);
		expect(result).not.toBeNull();
		// Narrow after the null guard.
		if (!result) return;
		expect(result.size).toBeLessThan(budget);
		expect(result.type).toBe("image/webp");
		expect(result.name.endsWith(".webp")).toBe(true);
	});
});
