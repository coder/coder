// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

export function getStorage<T>(
  storage: Storage,
  key: string,
  validate: (value: unknown) => T,
) {
  try {
    const item = storage.getItem(key);

    if (!item) {
      throw new Error(`No value found for "${key}".`);
    }

    const value = JSON.parse(item) as T;

    return validate(value);
  } catch {
    return null;
  }
}

export function setStorage<T>(storage: Storage, key: string, value: T) {
  storage.setItem(key, JSON.stringify(value));
}
