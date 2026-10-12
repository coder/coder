const lastReloadKey = "preload-reload";
const reloadGuardMs = 10_000;

let reloadPending = false;

/**
 * Reloads the page when Vite fails to load a chunk. After a redeploy the SPA
 * still references the previous build's chunk filenames, which the server no
 * longer has. A reload fetches index.html with the new filenames. A
 * sessionStorage guard prevents reload loops when the failure persists.
 */
export function registerStaleChunkReload(): () => void {
	const onPreloadError = () => {
		const last = sessionStorage.getItem(lastReloadKey);
		const now = Date.now();
		if (last && now - Number(last) <= reloadGuardMs) {
			return;
		}
		sessionStorage.setItem(lastReloadKey, String(now));
		reloadPending = true;
		location.reload();
	};
	window.addEventListener("vite:preloadError", onPreloadError);
	return () => window.removeEventListener("vite:preloadError", onPreloadError);
}

/**
 * Reports whether a stale-chunk reload is underway. Vite dispatches
 * `vite:preloadError` synchronously before rethrowing the import error, so
 * this is already true when the failed import reaches an error boundary.
 */
export function isStaleChunkReloadPending(): boolean {
	return reloadPending;
}
