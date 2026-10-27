const RELOAD_TIMESTAMP_STORAGE_KEY = "preload-reload";
const RELOAD_GUARD_MS = 10_000;

let reloadPending = false;
let prefetches = 0;

/**
 * Reloads the page when Vite fails to load a chunk. After a redeploy the SPA
 * still references the previous build's chunk filenames, which the server no
 * longer has. A reload fetches index.html with the new filenames. A
 * sessionStorage guard prevents reload loops when the failure persists.
 */
export function registerStaleChunkReload(): void {
	window.addEventListener("vite:preloadError", () => {
		if (prefetches > 0) {
			return;
		}
		const lastReloadAt = sessionStorage.getItem(RELOAD_TIMESTAMP_STORAGE_KEY);
		const now = Date.now();

		if (lastReloadAt && now - Number(lastReloadAt) <= RELOAD_GUARD_MS) {
			return;
		}

		sessionStorage.setItem(RELOAD_TIMESTAMP_STORAGE_KEY, String(now));
		reloadPending = true;

		location.reload();
	});
}

/**
 * Reports whether a stale-chunk reload is underway. Vite dispatches
 * `vite:preloadError` synchronously before rethrowing the import error, so
 * this is already true when the failed import reaches an error boundary.
 */
export function isStaleChunkReloadPending(): boolean {
	return reloadPending;
}

/**
 * Loads a chunk the page may need later, such as on hover, and ignores a
 * failure without reloading: the import that needs the chunk fails again and
 * reloads then. Other chunk failures while a prefetch runs also skip the
 * reload, so start the import that needs the chunk after this settles.
 */
export async function prefetchChunk(
	load: () => Promise<unknown>,
): Promise<void> {
	prefetches++;
	try {
		await load();
	} catch {
		// The import that needs the chunk reports the failure.
	} finally {
		prefetches--;
	}
}
