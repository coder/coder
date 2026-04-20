/**
 * Browser-side image resizing for provider-specific payload budgets.
 *
 * Motivation: Anthropic rejects inline images over 5,242,880 bytes
 * (5 MiB). Our upload endpoint otherwise accepts images up to 10 MiB,
 * so oversized images would reach the model and fail. Resizing on the
 * client avoids having the server decode attacker-controlled image
 * bytes (image-bomb DoS surface).
 *
 * This module is plain TS — no React — so it can be called from any
 * upload pipeline.
 */

// Formats we know how to safely re-encode. GIFs are intentionally
// excluded: canvas re-encoding would flatten animation to a single
// frame, which is worse than failing the budget check.
const RESIZABLE_MIME_TYPES = new Set([
	"image/png",
	"image/jpeg",
	"image/jpg",
	"image/webp",
]);

// Canvas dimensions are clamped here because several browsers limit
// the maximum decoded canvas area (Safari historically ~4096² on
// mobile, Chrome ~16384 per side on desktop). 8192 is a safe
// conservative lower bound that still preserves plenty of detail for
// typical screenshots.
const MAX_INITIAL_DIMENSION = 8192;

// Maximum number of shrink iterations before we give up. Each
// iteration multiplies dimensions by DIMENSION_STEP, so after N
// iterations the image is ~DIMENSION_STEP^N of the original on each
// axis. 8 iterations → ~17% of original dimensions → ~3% of original
// pixels, which is plenty of headroom for any image that was only
// modestly over budget.
const MAX_SHRINK_ITERATIONS = 8;
const DIMENSION_STEP = 0.8;

// Initial and fallback WebP quality parameters. WebP at 0.85 is
// visually near-lossless for screenshots; 0.7 is the hail-mary that
// runs only if dimension shrinking alone didn't fit the budget.
const INITIAL_QUALITY = 0.85;
const FALLBACK_QUALITY = 0.7;

// Sequential queue so pasting or dropping many images doesn't spawn
// many simultaneous decode pipelines. Each call waits for the
// previous to finish before starting.
let queue: Promise<unknown> = Promise.resolve();

function enqueue<T>(fn: () => Promise<T>): Promise<T> {
	const next = queue.then(fn, fn);
	// Swallow rejection on the chain itself so later calls don't
	// inherit it; the caller still sees the original rejection.
	queue = next.catch(() => undefined);
	return next;
}

/**
 * Re-encode `file` as WebP, iteratively shrinking until the output
 * is strictly smaller than `maxBytes`.
 *
 * - Returns the original `file` unchanged when it is already under
 *   the budget and its MIME type is in our resizable set (no need
 *   to pay the decode cost).
 * - Returns the original `file` unchanged for animated formats like
 *   GIF where canvas re-encoding would destroy the animation.
 * - Returns a new `File` (WebP, renamed to `.webp`) when resizing
 *   succeeded.
 * - Returns `null` when the file cannot be decoded or no iteration
 *   fit the budget; callers fall back to the original file.
 */
export async function resizeImageToMaxBytes(
	file: File,
	maxBytes: number,
): Promise<File | null> {
	if (!file.type.startsWith("image/")) {
		return file;
	}
	// Animated formats — return as-is rather than silently producing
	// a single-frame still.
	if (file.type === "image/gif") {
		return file;
	}
	// Fast path: already under budget in a format the server allows.
	if (file.size < maxBytes && RESIZABLE_MIME_TYPES.has(file.type)) {
		return file;
	}
	if (!RESIZABLE_MIME_TYPES.has(file.type)) {
		return null;
	}

	return enqueue(() => shrinkOnce(file, maxBytes));
}

async function shrinkOnce(file: File, maxBytes: number): Promise<File | null> {
	let bitmap: ImageBitmap | null = null;
	try {
		bitmap = await decodeToBitmap(file);
	} catch {
		return null;
	}
	if (!bitmap) {
		return null;
	}

	try {
		let width = Math.min(bitmap.width, MAX_INITIAL_DIMENSION);
		let height = Math.min(bitmap.height, MAX_INITIAL_DIMENSION);
		if (width <= 0 || height <= 0) {
			return null;
		}
		// Preserve aspect ratio when the source exceeds the clamp.
		const ratio = bitmap.width / bitmap.height;
		if (bitmap.width > MAX_INITIAL_DIMENSION) {
			width = MAX_INITIAL_DIMENSION;
			height = Math.max(1, Math.round(MAX_INITIAL_DIMENSION / ratio));
		}
		if (bitmap.height > MAX_INITIAL_DIMENSION) {
			height = MAX_INITIAL_DIMENSION;
			width = Math.max(1, Math.round(MAX_INITIAL_DIMENSION * ratio));
		}

		for (let i = 0; i < MAX_SHRINK_ITERATIONS; i++) {
			const blob = await encodeWebP(bitmap, width, height, INITIAL_QUALITY);
			if (blob && blob.size < maxBytes) {
				return toWebPFile(file, blob);
			}
			// Guard against tiny images that can't shrink further.
			if (width <= 1 || height <= 1) {
				break;
			}
			width = Math.max(1, Math.round(width * DIMENSION_STEP));
			height = Math.max(1, Math.round(height * DIMENSION_STEP));
		}

		// Last-ditch attempt at the smallest dimensions with a lower
		// quality setting. Useful for mostly-photographic images where
		// dimension shrinking alone saturated.
		const fallbackBlob = await encodeWebP(
			bitmap,
			width,
			height,
			FALLBACK_QUALITY,
		);
		if (fallbackBlob && fallbackBlob.size < maxBytes) {
			return toWebPFile(file, fallbackBlob);
		}
		return null;
	} catch {
		return null;
	} finally {
		bitmap.close?.();
	}
}

async function decodeToBitmap(file: File): Promise<ImageBitmap | null> {
	// createImageBitmap is the fastest path and avoids ever drawing
	// through an <img> tag (which would require DOM attachment in
	// some browsers).
	if (typeof createImageBitmap === "function") {
		return await createImageBitmap(file);
	}
	// Fallback: decode via <img> + Blob URL. Only reached on very
	// old browsers and in some test environments.
	return await new Promise<ImageBitmap | null>((resolve, reject) => {
		const url = URL.createObjectURL(file);
		const img = new Image();
		img.onload = () => {
			URL.revokeObjectURL(url);
			// Best-effort cast: HTMLImageElement is acceptable as a
			// CanvasImageSource and we only need width/height plus the
			// drawable surface below.
			resolve(img as unknown as ImageBitmap);
		};
		img.onerror = () => {
			URL.revokeObjectURL(url);
			reject(new Error("image decode failed"));
		};
		img.src = url;
	});
}

async function encodeWebP(
	source: ImageBitmap,
	width: number,
	height: number,
	quality: number,
): Promise<Blob | null> {
	// Prefer OffscreenCanvas when available — its convertToBlob is
	// fully async and does not require the canvas to be laid out.
	if (typeof OffscreenCanvas === "function") {
		const canvas = new OffscreenCanvas(width, height);
		const ctx = canvas.getContext("2d");
		if (!ctx) {
			return null;
		}
		ctx.drawImage(source, 0, 0, width, height);
		try {
			return await canvas.convertToBlob({ type: "image/webp", quality });
		} catch {
			return null;
		}
	}

	// HTMLCanvasElement fallback for environments without
	// OffscreenCanvas (older Safari, some embedded webviews).
	const canvas = document.createElement("canvas");
	canvas.width = width;
	canvas.height = height;
	const ctx = canvas.getContext("2d");
	if (!ctx) {
		return null;
	}
	ctx.drawImage(source as CanvasImageSource, 0, 0, width, height);
	return await new Promise<Blob | null>((resolve) => {
		canvas.toBlob((blob) => resolve(blob), "image/webp", quality);
	});
}

function toWebPFile(original: File, blob: Blob): File {
	// Replace the extension (if any) with .webp so the filename
	// matches the new content type. File browsers sometimes key
	// handling off the extension even when the MIME is correct.
	const dot = original.name.lastIndexOf(".");
	const baseName = dot > 0 ? original.name.slice(0, dot) : original.name;
	const webpName = `${baseName || "image"}.webp`;
	return new File([blob], webpName, {
		type: "image/webp",
		lastModified: original.lastModified,
	});
}
