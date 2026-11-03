// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

export function noop(..._: unknown[]) {
  return;
}

export async function noopAsync(..._: unknown[]) {
  return;
}
