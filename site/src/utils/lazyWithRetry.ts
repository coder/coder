import { type ComponentType, lazy } from "react";

/**
 * Wraps React.lazy() with retry logic for transient chunk-loading
 * failures. Retries up to 3 times with exponential backoff
 * (1s, 2s, 4s) before propagating the error.
 */
export function lazyWithRetry<P>(
	factory: () => Promise<{ default: ComponentType<P> }>,
	maxRetries = 3,
): React.LazyExoticComponent<ComponentType<P>> {
	return lazy(async () => {
		let lastError: unknown;
		for (let attempt = 0; attempt <= maxRetries; attempt++) {
			try {
				return await factory();
			} catch (err) {
				lastError = err;
				if (attempt < maxRetries && isTransientImportError(err)) {
					const delay = 2 ** attempt * 1000;
					await new Promise((r) => setTimeout(r, delay));
				} else {
					throw err;
				}
			}
		}
		throw lastError;
	});
}

/**
 * Heuristic for transient chunk-load / network errors that are worth
 * retrying. Deterministic failures (syntax errors, missing exports)
 * are not retried.
 */
function isTransientImportError(err: unknown): boolean {
	if (!(err instanceof Error)) {
		return false;
	}
	const msg = err.message.toLowerCase();
	return (
		msg.includes("failed to fetch") ||
		msg.includes("loading chunk") ||
		msg.includes("loading css chunk") ||
		msg.includes("dynamically imported module") ||
		msg.includes("networkerror") ||
		msg.includes("load failed")
	);
}
