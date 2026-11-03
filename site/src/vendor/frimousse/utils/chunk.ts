// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

export function chunk<T>(array: T[], size: number): T[][] {
  const chunks: T[][] = [];

  if (size <= 0) {
    return chunks;
  }

  for (let i = 0, j = array.length; i < j; i += size) {
    chunks.push(array.slice(i, i + size));
  }

  return chunks;
}
