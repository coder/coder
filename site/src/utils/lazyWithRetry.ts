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
				if (attempt < maxRetries) {
					const delay = 2 ** attempt * 1000;
					await new Promise((r) => setTimeout(r, delay));
				}
			}
		}
		throw lastError;
	});
}
