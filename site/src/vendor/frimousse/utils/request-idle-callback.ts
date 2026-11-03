// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

const DEFAULT_TIMEOUT = 10;
const DEFAULT_MAX_TIMEOUT = 50;

// Safari doesn't support requestIdleCallback yet
export function requestIdleCallback(
  callback: IdleRequestCallback,
  options?: IdleRequestOptions,
) {
  let id: ReturnType<typeof window.requestIdleCallback> | null = null;

  if (typeof window.requestIdleCallback === "function") {
    id = window.requestIdleCallback(callback, options);
  } else {
    const start = Date.now();

    id = window.setTimeout(() => {
      callback({
        didTimeout: false,
        timeRemaining: () =>
          Math.max(
            0,
            (options?.timeout ?? DEFAULT_MAX_TIMEOUT) - (Date.now() - start),
          ),
      });
    }, DEFAULT_TIMEOUT);
  }

  return () => {
    if (typeof window.cancelIdleCallback === "function") {
      window.cancelIdleCallback(id);
    } else {
      window.clearTimeout(id);
    }
  };
}
