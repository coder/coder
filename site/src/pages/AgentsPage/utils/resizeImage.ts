/**
 * Browser-side image re-encoding to a caller-supplied byte budget.
 * Plain TS (no React) so it can be used by any upload pipeline.
 */

// Formats we re-encode. GIFs are excluded so we don't flatten
// animation; image/jpg is a non-IANA alias for image/jpeg some
// OSes emit.
const RESIZABLE_MIME_TYPES = new Set([
	"image/png",
	"image/jpeg",
	"image/jpg",
	"image/webp",
]);

// Per-axis clamp applied at decode so a low-byte image with
// pathologically large pixel extent can't OOM the tab. 8192 stays
// under Safari/Chrome canvas limits while preserving detail for
// typical screenshots.
const MAX_INITIAL_DIMENSION = 8192;

// 8 iterations × DIMENSION_STEP per axis gives ~3% of original
// pixels, plenty of headroom for any modestly-oversize image.
const MAX_SHRINK_ITERATIONS = 8;
const DIMENSION_STEP = 0.8;

// 0.85 is near-lossless for screenshots; 0.7 is a hail-mary if
// dimension shrinking alone didn't fit the budget.
const INITIAL_QUALITY = 0.85;
const FALLBACK_QUALITY = 0.7;

// Time-bound the legacy <img> decode fallback so a blob that fires
// neither onload nor onerror can't wedge the module queue.
const FALLBACK_DECODE_TIMEOUT_MS = 10_000;

// Sequential queue: pasting many images won't spawn parallel
// decode pipelines.
let queue: Promise<unknown> = Promise.resolve();

function enqueue<T>(fn: () => Promise<T>): Promise<T> {
	// Chain off both settlement branches so the next task runs
	// after a rejection too; the .catch below detaches rejection
	// from the shared tail.
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
	// GIFs return as-is so we don't flatten animation.
	if (file.type === "image/gif") {
		return file;
	}
	// Already under budget; return as-is regardless of MIME (the
	// function's contract is "give me something <= maxBytes" and
	// we already have that).
	if (file.size <= maxBytes) {
		return file;
	}
	// Over budget but unsupported MIME (e.g. image/bmp): refuse
	// rather than silently produce a black canvas or wrong file.
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
		// decodeToBitmap already clamped to MAX_INITIAL_DIMENSION
		// per axis, so we start the shrink loop from the bitmap's
		// reported dimensions.
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

		// Last-ditch attempt at the smallest dimensions with a
		// lower quality, for photographic images where dimension
		// shrinking alone saturated.
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
	// createImageBitmap's HTML-spec output rules:
	//   - both resize dims => stretch (destroys aspect ratio).
	//   - only resizeWidth => width is exact, height proportional.
	//     Per spec this UPSCALES if resizeWidth > natural width.
	//   - both omitted => source's natural size.
	//
	// Upscaling can blow past Chromium's ~268M-pixel decode limit
	// (e.g. a 1080x5000 screenshot with resizeWidth: 8192 becomes
	// ~310M pixels). Probe natural dimensions first to pick the
	// smallest resize option that fits.
	if (typeof createImageBitmap !== "function") {
		return await decodeViaImgFallback(file);
	}
	const natural = await probeNaturalDimensions(file);
	if (!natural) {
		return await decodeViaImgFallback(file);
	}
	const { width, height } = natural;
	if (width <= 0 || height <= 0) {
		return null;
	}
	// Pick the smallest resize that fits MAX_INITIAL_DIMENSION
	// without upscaling. Already-small sources pass no options.
	const needsWidthClamp = width > MAX_INITIAL_DIMENSION;
	const needsHeightClamp = height > MAX_INITIAL_DIMENSION;
	if (!needsWidthClamp && !needsHeightClamp) {
		return await createImageBitmap(file);
	}
	// Clamp the longer axis; the shorter axis scales with it.
	if (width >= height) {
		return await createImageBitmap(file, {
			resizeWidth: MAX_INITIAL_DIMENSION,
			resizeQuality: "medium",
		});
	}
	return await createImageBitmap(file, {
		resizeHeight: MAX_INITIAL_DIMENSION,
		resizeQuality: "medium",
	});
}

// Reads natural dimensions via <img> without allocating a full
// ImageBitmap. Returns null on decode/timeout failure.
async function probeNaturalDimensions(
	file: File,
): Promise<{ width: number; height: number } | null> {
	return await new Promise<{ width: number; height: number } | null>(
		(resolve) => {
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
				resolve(null);
			}, FALLBACK_DECODE_TIMEOUT_MS);
			img.onload = () => {
				if (settled) return;
				clearTimeout(timer);
				cleanup();
				resolve({ width: img.naturalWidth, height: img.naturalHeight });
			};
			img.onerror = () => {
				if (settled) return;
				clearTimeout(timer);
				cleanup();
				resolve(null);
			};
			img.src = url;
		},
	);
}

async function decodeViaImgFallback(file: File): Promise<ImageBitmap | null> {
	// Decode via <img> + Blob URL. Reached only on browsers
	// without createImageBitmap (very old Safari, embedded
	// webviews); time-bounded so a stuck decoder can't wedge the
	// queue. No decode-time clamp on this path.
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
			// HTMLImageElement is a valid CanvasImageSource;
			// width/height are all we need downstream.
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
	// OffscreenCanvas's convertToBlob is fully async and doesn't
	// need the canvas laid out.
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

	// Fallback for environments without OffscreenCanvas.
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
	// Match the extension to the actual content type; some upload
	// handlers still key behavior off the extension.
	const dot = original.name.lastIndexOf(".");
	const baseName = dot > 0 ? original.name.slice(0, dot) : original.name;
	const webpName = `${baseName || "image"}.webp`;
	// Use blob.type: canvas encoders fall back to PNG on browsers
	// without WebP support; this keeps the File's labelled type
	// matching its actual content.
	const effectiveType = blob.type || "image/webp";
	const effectiveName =
		effectiveType === "image/webp" ? webpName : original.name;
	return new File([blob], effectiveName, {
		type: effectiveType,
		lastModified: original.lastModified,
	});
}
