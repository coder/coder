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
		// Canvas re-encoding flattens animation; we hand the
		// original back instead.
		const bytes = new Uint8Array(8 * 1024 * 1024);
		const file = new File([bytes], "clip.gif", { type: "image/gif" });
		const result = await resizeImageToMaxBytes(file, 1024 * 1024);
		expect(result).toBe(file);
	});

	it("returns the original image unchanged when already under budget", async () => {
		const bytes = new Uint8Array(1024);
		const file = new File([bytes], "tiny.png", { type: "image/png" });
		const result = await resizeImageToMaxBytes(file, 4096);
		expect(result).toBe(file);
	});

	it("returns null for an unsupported image MIME that would need resizing", async () => {
		// Unsupported MIME + over budget: refuse rather than
		// silently produce garbage.
		const bytes = new Uint8Array(2 * 1024 * 1024);
		const file = new File([bytes], "diagram.svg", {
			type: "image/svg+xml",
		});
		const result = await resizeImageToMaxBytes(file, 1024 * 1024);
		expect(result).toBeNull();
	});

	it("accepts image/jpg alias alongside image/jpeg", async () => {
		// Pins the non-IANA `image/jpg` alias in RESIZABLE_MIME_TYPES.
		// The under-budget passthrough is enough to prove acceptance;
		// the over-budget case in the stubbed-decoder block proves
		// the encode pipeline runs.
		const under = new File([new Uint8Array(512)], "icon.jpg", {
			type: "image/jpg",
		});
		const result = await resizeImageToMaxBytes(under, 4096);
		expect(result).toBe(under);
	});

	it("returns an under-budget unsupported-MIME image unchanged", async () => {
		// Under-budget unsupported MIMEs pass through; the contract
		// is "give me something <= maxBytes" and we already have
		// that.
		const bytes = new Uint8Array(512);
		const file = new File([bytes], "icon.bmp", {
			type: "image/bmp",
		});
		const result = await resizeImageToMaxBytes(file, 4096);
		expect(result).toBe(file);
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

		// Fake createImageBitmap matching the HTML spec output rules:
		//   - both resize dims => stretch (no aspect-ratio preservation).
		//   - only resizeWidth => width exact, height proportional.
		//     UPSCALES if resizeWidth > source width.
		//   - only resizeHeight => mirror of above.
		//   - neither => source dimensions unchanged.
		//
		// Critical that the fake doesn't cap at source dimensions:
		// real browsers follow the spec and upscale, so production
		// code must handle that. A capped fake would mask the bug.
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
					const srcW = state.srcWidth;
					const srcH = state.srcHeight;
					const rW = options?.resizeWidth;
					const rH = options?.resizeHeight;
					let w: number;
					let h: number;
					if (rW !== undefined && rH !== undefined) {
						// Spec: stretch-to-fit, no source clamp.
						w = rW;
						h = rH;
					} else if (rW !== undefined) {
						w = rW;
						h = Math.max(1, Math.round((srcH * rW) / srcW));
					} else if (rH !== undefined) {
						h = rH;
						w = Math.max(1, Math.round((srcW * rH) / srcH));
					} else {
						w = srcW;
						h = srcH;
					}
					return {
						width: w,
						height: h,
						close: vi.fn(),
					} as unknown as ImageBitmap;
				},
			),
		);

		// Fake Image (for probeNaturalDimensions in production
		// code). jsdom exposes `Image` but does not load blob URLs,
		// so we stub it with a synthetic implementation that reports
		// state.srcWidth/Height and fires onload on microtask.
		class FakeImage {
			onload: (() => void) | null = null;
			onerror: (() => void) | null = null;
			naturalWidth = 0;
			naturalHeight = 0;
			private _src = "";
			get src() {
				return this._src;
			}
			set src(url: string) {
				this._src = url;
				queueMicrotask(() => {
					if (state.decodeThrows) {
						this.onerror?.();
						return;
					}
					this.naturalWidth = state.srcWidth;
					this.naturalHeight = state.srcHeight;
					this.onload?.();
				});
			}
		}
		vi.stubGlobal("Image", FakeImage);

		// convertToBlob size scales with width*height*quality so
		// the shrink loop converges.
		class FakeOffscreenCanvas {
			width: number;
			height: number;
			constructor(w: number, h: number) {
				this.width = w;
				this.height = h;
			}
			getContext() {
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
		// Forces at least a few shrink iterations before convergence.
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

		expect(state.encodeCalls.length).toBeGreaterThan(0);
	});

	it("decoder never stretches non-square sources", async () => {
		// 4:1 source with long axis above MAX_INITIAL_DIMENSION.
		// If decodeToBitmap passes both resize dims, the spec
		// would force 8192x8192 output and the ratio assertion
		// would fail.
		state.srcWidth = 16_000;
		state.srcHeight = 4_000;
		const file = new File([new Uint8Array(6 * 1024 * 1024)], "wide.png", {
			type: "image/png",
		});
		await resizeImageToMaxBytes(file, 32 * 1024);
		expect(state.encodeCalls.length).toBeGreaterThan(0);
		const first = state.encodeCalls[0];
		const ratio = first.width / first.height;
		expect(ratio).toBeGreaterThanOrEqual(4 - 0.05);
		expect(ratio).toBeLessThanOrEqual(4 + 0.05);
		expect(first.width).toBeLessThanOrEqual(8192);
		expect(first.height).toBeLessThanOrEqual(8192);
	});

	it("keeps the bitmap within MAX_INITIAL_DIMENSION on both axes for extreme portraits", async () => {
		// Regression: passing resizeWidth: MAX on a 2000x60000
		// source would upscale to 8192x245760, blowing past
		// Chromium's ~268M pixel limit. Probe must pick
		// resizeHeight only.
		state.srcWidth = 2000;
		state.srcHeight = 60_000;
		const file = new File([new Uint8Array(6 * 1024 * 1024)], "tall.png", {
			type: "image/png",
		});
		await resizeImageToMaxBytes(file, 64 * 1024);
		for (const call of state.encodeCalls) {
			expect(call.width).toBeLessThanOrEqual(8192);
			expect(call.height).toBeLessThanOrEqual(8192);
		}
		const first = state.encodeCalls[0];
		const sourceRatio = 2000 / 60_000;
		const bitmapRatio = first.width / first.height;
		expect(bitmapRatio).toBeGreaterThan(sourceRatio * 0.99);
		expect(bitmapRatio).toBeLessThan(sourceRatio * 1.01);
	});

	it("stays within MAX_INITIAL_DIMENSION when source exceeds it on both axes", async () => {
		state.srcWidth = 60_000;
		state.srcHeight = 40_000;
		const file = new File([new Uint8Array(6 * 1024 * 1024)], "huge.png", {
			type: "image/png",
		});
		await resizeImageToMaxBytes(file, 256 * 1024);
		for (const call of state.encodeCalls) {
			expect(call.width).toBeLessThanOrEqual(8192);
			expect(call.height).toBeLessThanOrEqual(8192);
		}
	});

	it("skips createImageBitmap resize options entirely when source is already under clamp", async () => {
		// Regression: passing resizeWidth: MAX on a 1920x1080
		// source would upscale to 8192x4608 (spec: output width
		// is exactly resizeWidth). Probe must skip resize options
		// when the source already fits.
		state.srcWidth = 1920;
		state.srcHeight = 1080;
		const file = new File([new Uint8Array(6 * 1024 * 1024)], "shot.png", {
			type: "image/png",
		});
		await resizeImageToMaxBytes(file, 256 * 1024);
		const first = state.encodeCalls[0];
		expect(first.width).toBe(1920);
		expect(first.height).toBe(1080);
	});

	it("tries the fallback quality pass when shrink iterations saturate", async () => {
		// Tiny source + unreachable 1-byte budget exhausts the main
		// loop and forces FALLBACK_QUALITY (0.7).
		state.srcWidth = 64;
		state.srcHeight = 64;
		const file = new File([new Uint8Array(1024 * 1024)], "tiny.png", {
			type: "image/png",
		});
		const result = await resizeImageToMaxBytes(file, 1);
		expect(result).toBeNull();
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

	it("fake createImageBitmap matches HTML spec output-dimension rules", async () => {
		// Pins fake createImageBitmap behavior: stretch with both
		// dims, proportional scale with one, including upscale
		// when a resize dim exceeds the source. A fake that capped
		// at source dimensions or scaled uniformly would mask real
		// decoder bugs in production code.
		state.srcWidth = 4000;
		state.srcHeight = 1000;
		const blob = new Blob([new Uint8Array(8)], { type: "image/png" });
		const createBitmap = (
			globalThis as unknown as {
				createImageBitmap: (
					blob: Blob,
					opts?: { resizeWidth?: number; resizeHeight?: number },
				) => Promise<ImageBitmap>;
			}
		).createImageBitmap;

		// Stretch.
		const stretched = await createBitmap(blob, {
			resizeWidth: 800,
			resizeHeight: 800,
		});
		expect(stretched.width).toBe(800);
		expect(stretched.height).toBe(800);

		// Downscale by width.
		const downWidth = await createBitmap(blob, { resizeWidth: 800 });
		expect(downWidth.width).toBe(800);
		expect(downWidth.height).toBe(200);

		// Downscale by height.
		const downHeight = await createBitmap(blob, { resizeHeight: 200 });
		expect(downHeight.height).toBe(200);
		expect(downHeight.width).toBe(800);

		// Upscale by width: spec allows; a capped fake would
		// report (4000, 1000) and mask production decoder bugs.
		const upWidth = await createBitmap(blob, { resizeWidth: 8000 });
		expect(upWidth.width).toBe(8000);
		expect(upWidth.height).toBe(2000);

		// Natural.
		const natural = await createBitmap(blob);
		expect(natural.width).toBe(4000);
		expect(natural.height).toBe(1000);
	});

	it("keeps the File type honest when the encoder falls back to PNG", async () => {
		// Some browsers without WebP encode return a PNG; the
		// File's labelled type must match the actual content.
		state.convertBlobType = "image/png";
		const file = new File([new Uint8Array(2 * 1024 * 1024)], "photo.png", {
			type: "image/png",
		});
		const result = await resizeImageToMaxBytes(file, 1024 * 1024);
		expect(result).not.toBeNull();
		if (!result) return;
		expect(result.type).toBe("image/png");
		expect(result.name.endsWith(".webp")).toBe(false);
	});

	it("re-encodes an oversized image/jpg through the resize pipeline", async () => {
		// Over-budget: ensures the image/jpg alias passes the
		// allowlist gate and enters the encode pipeline. Removing
		// the alias from RESIZABLE_MIME_TYPES would short-circuit
		// to null here.
		const file = new File([new Uint8Array(3 * 1024 * 1024)], "photo.jpg", {
			type: "image/jpg",
		});
		const result = await resizeImageToMaxBytes(file, 512 * 1024);
		expect(result).not.toBeNull();
		expect(state.encodeCalls.length).toBeGreaterThan(0);
		if (!result) return;
		expect(result.size).toBeLessThanOrEqual(512 * 1024);
	});
});

describeIfDecode("resizeImageToMaxBytes with real decoders", () => {
	it("returns null (does not throw) for a corrupt image blob", async () => {
		// Real decoder only; jsdom <img> fallback never fires
		// onload/onerror on a corrupt blob and would hang.
		const bytes = new Uint8Array([0x00, 0x01, 0x02, 0x03]);
		const file = new File([bytes], "broken.png", { type: "image/png" });
		const result = await resizeImageToMaxBytes(file, 1);
		expect(result).toBeNull();
	});

	it("re-encodes a large PNG down to the requested byte budget", async () => {
		const canvas = new OffscreenCanvas(1024, 1024);
		const ctx = canvas.getContext("2d");
		if (!ctx) throw new Error("no 2d ctx");
		// Noise pattern so compression doesn't trivialize the
		// byte count before the shrink path runs.
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
		if (!result) return;
		expect(result.size).toBeLessThan(budget);
		expect(result.type).toBe("image/webp");
		expect(result.name.endsWith(".webp")).toBe(true);
	});
});
