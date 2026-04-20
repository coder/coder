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
// frame, which is worse than failing the budget check. `image/jpg`
// is a non-IANA alias some OSes/uploaders emit for .jpg files; we
// accept it as an alias for `image/jpeg`.
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
// typical screenshots. Applied at the createImageBitmap boundary so
// the raw decoded bitmap never exceeds the clamp — a low-byte image
// with pathologically large pixel extent therefore cannot OOM the
// tab on decode.
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

// Timeout for the legacy <img>-based decode fallback. The fallback
// is only used when createImageBitmap isn't available (very old
// Safari, some embedded webviews, some test envs); capping it here
// prevents a pathological blob that fires neither onload nor
// onerror from wedging the module queue for the tab's lifetime.
const FALLBACK_DECODE_TIMEOUT_MS = 10_000;

// Sequential queue so pasting or dropping many images doesn't spawn
// many simultaneous decode pipelines. Each call waits for the
// previous to finish before starting.
let queue: Promise<unknown> = Promise.resolve();

function enqueue<T>(fn: () => Promise<T>): Promise<T> {
	// Chain fn off both settlement branches so the next queued task
	// runs regardless of whether the previous one resolved or
	// rejected; the .catch below then detaches rejection from the
	// shared tail so later calls don't inherit a poisoned promise.
	const next = queue.then(fn, fn);
	queue = next.catch(() => undefined);
	return next;
}

/**
 * Re-encode `file` as WebP, iteratively shrinking until the output
 * is at or below `maxBytes`.
 *
 * - Returns the original `file` unchanged when it is already within
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
	// Fast path: already within budget in a format the server allows.
	// Uses <= to match the caller's gate (useFileAttachments) and the
	// server-side cap predicate, so a file at exactly `maxBytes` is
	// consistently treated as "already fits".
	if (file.size <= maxBytes && RESIZABLE_MIME_TYPES.has(file.type)) {
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
		// createImageBitmap with resizeWidth/resizeHeight has already
		// clamped the bitmap to at most MAX_INITIAL_DIMENSION per
		// axis (see decodeToBitmap), preserving aspect ratio. We can
		// therefore start the shrink loop directly at the bitmap's
		// reported dimensions without a separate manual clamp.
		let width = bitmap.width;
		let height = bitmap.height;
		if (width <= 0 || height <= 0) {
			return null;
		}

		for (let i = 0; i < MAX_SHRINK_ITERATIONS; i++) {
			const blob = await encodeWebP(bitmap, width, height, INITIAL_QUALITY);
			if (blob && blob.size <= maxBytes) {
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
		if (fallbackBlob && fallbackBlob.size <= maxBytes) {
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
	// some browsers). The resize* options clamp the decoded bitmap
	// before it reaches memory, so a small file with pathological
	// pixel extent cannot blow up RAM here.
	if (typeof createImageBitmap === "function") {
		return await createImageBitmap(file, {
			resizeWidth: MAX_INITIAL_DIMENSION,
			resizeHeight: MAX_INITIAL_DIMENSION,
			resizeQuality: "medium",
		});
	}
	// Fallback: decode via <img> + Blob URL. Only reached on very
	// old browsers and in some test environments. This path does
	// not get the decode-time clamp, but it is intentionally
	// time-bounded so a stuck decoder can't wedge the shared queue.
	return await new Promise<ImageBitmap | null>((resolve, reject) => {
		const url = URL.createObjectURL(file);
		const img = new Image();
		let settled = false;
		const cleanup = () => {
			settled = true;
			URL.revokeObjectURL(url);
		};
		const timer = setTimeout(() => {
			if (settled) return;
			cleanup();
			reject(new Error("image decode timed out"));
		}, FALLBACK_DECODE_TIMEOUT_MS);
		img.onload = () => {
			if (settled) return;
			clearTimeout(timer);
			cleanup();
			// Best-effort cast: HTMLImageElement is acceptable as a
			// CanvasImageSource and we only need width/height plus the
			// drawable surface below.
			resolve(img as unknown as ImageBitmap);
		};
		img.onerror = () => {
			if (settled) return;
			clearTimeout(timer);
			cleanup();
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
	// matches the new content type. Some web upload handlers and
	// file-picker dialogs still key behavior off the extension even
	// when the MIME type is correct, so keeping them consistent
	// avoids surprises.
	const dot = original.name.lastIndexOf(".");
	const baseName = dot > 0 ? original.name.slice(0, dot) : original.name;
	const webpName = `${baseName || "image"}.webp`;
	// Prefer the blob's reported MIME type. canvas.toBlob/
	// convertToBlob fall back to image/png on browsers without WebP
	// encode support; trusting the browser's label keeps the File
	// type honest instead of labelling a PNG as WebP.
	const effectiveType = blob.type || "image/webp";
	const effectiveName =
		effectiveType === "image/webp" ? webpName : original.name;
	return new File([blob], effectiveName, {
		type: effectiveType,
		lastModified: original.lastModified,
	});
}
