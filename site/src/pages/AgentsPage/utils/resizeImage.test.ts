import { describe, expect, it } from "vitest";
import { resizeImageToMaxBytes } from "./resizeImage";

// jsdom (the default vitest environment) does not implement
// createImageBitmap / OffscreenCanvas, so the re-encode codepaths
// cannot run here. Tests that exercise them are gated behind this
// flag and will be skipped in jsdom; the rest cover the pure-logic
// fast paths and failure handling.
const canDecodeImages =
	typeof createImageBitmap === "function" &&
	typeof OffscreenCanvas === "function";

const describeIfDecode = canDecodeImages ? describe : describe.skip;

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
