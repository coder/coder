// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import type {
  EmojiData,
  EmojiDataResolver,
  EmojiDataResolverOptions,
  Locale,
} from "../types";

type EmojiDataSourceOptions = EmojiDataResolverOptions & {
  resolveEmojiData?: EmojiDataResolver;
};

type EmojiDataEntry = {
  data?: EmojiData;
  load?:
    | { status: "pending"; promise: Promise<void>; complete: () => void }
    | { status: "error"; error: Error };
};

const loadedData = new Map<string, EmojiDataEntry>();
const customData = new WeakMap<
  EmojiDataResolver,
  Map<string, EmojiDataEntry>
>();
const subscribers = new Set<() => void>();

export function getEmojibaseUrl({
  emojiVersion,
  emojibaseUrl,
}: EmojiDataResolverOptions): string {
  return (
    emojibaseUrl ??
    (typeof emojiVersion === "number"
      ? `https://cdn.jsdelivr.net/npm/emojibase-data@${Math.floor(emojiVersion)}`
      : "https://cdn.jsdelivr.net/npm/emojibase-data@latest")
  );
}

export function getCachedEmojiData(
  locale: Locale,
  options: EmojiDataSourceOptions,
): EmojiData | undefined {
  const cache = options.resolveEmojiData
    ? customData.get(options.resolveEmojiData)
    : loadedData;

  return cache?.get(getKey(locale, options))?.data;
}

export function setCachedEmojiData(
  locale: Locale,
  options: EmojiDataSourceOptions,
  data: EmojiData,
): void {
  const entry = getEntry(locale, options);
  const pending = entry.load?.status === "pending" ? entry.load : undefined;
  entry.data = data;
  entry.load = undefined;
  pending?.complete();

  for (const subscriber of subscribers) {
    subscriber();
  }
}

/**
 * Suspended renders may never mount, so requests and errors outlive components.
 */
export function suspendForEmojiData(
  locale: Locale,
  options: EmojiDataSourceOptions,
  load: () => EmojiData | Promise<EmojiData>,
): never {
  const entry = getEntry(locale, options);

  if (entry.load?.status === "error") {
    throw entry.load.error;
  }

  if (!entry.load) {
    let complete = () => {};
    const promise = new Promise<void>((resolve) => {
      complete = resolve;
    });
    entry.load = { status: "pending", promise, complete };

    Promise.resolve()
      .then(load)
      .then(
        (data) => {
          if (!entry.data) {
            setCachedEmojiData(locale, options, data);
          }
        },
        (error: unknown) => {
          if (!entry.data) {
            entry.load = {
              status: "error",
              error:
                error instanceof Error
                  ? error
                  : new Error("Could not load emoji data", { cause: error }),
            };
          }

          complete();
        },
      );
  }

  throw entry.load.promise;
}

export function subscribeToEmojiData(subscriber: () => void): () => void {
  subscribers.add(subscriber);

  return () => {
    subscribers.delete(subscriber);
  };
}

export async function resolveCustomEmojiData(
  resolveEmojiData: EmojiDataResolver,
  locale: Locale,
  options: EmojiDataResolverOptions,
): Promise<EmojiData> {
  options.signal?.throwIfAborted();
  const data = await resolveEmojiData(locale, options);
  options.signal?.throwIfAborted();
  setCachedEmojiData(locale, { ...options, resolveEmojiData }, data);

  return data;
}

function getKey(locale: Locale, options: EmojiDataSourceOptions): string {
  return options.resolveEmojiData
    ? JSON.stringify([locale, options.emojibaseUrl, options.emojiVersion])
    : JSON.stringify([locale, getEmojibaseUrl(options)]);
}

function getEntry(
  locale: Locale,
  options: EmojiDataSourceOptions,
): EmojiDataEntry {
  let cache = loadedData;

  if (options.resolveEmojiData) {
    const existing = customData.get(options.resolveEmojiData);
    cache = existing ?? new Map();

    if (!existing) {
      customData.set(options.resolveEmojiData, cache);
    }
  }

  const key = getKey(locale, options);
  let entry = cache.get(key);

  if (!entry) {
    entry = {};
    cache.set(key, entry);
  }

  return entry;
}
