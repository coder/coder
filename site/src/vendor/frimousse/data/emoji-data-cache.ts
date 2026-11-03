// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import type { EmojiData, EmojiDataCache } from "../types";
import { getStorage, setStorage } from "../utils/storage";
import * as $ from "../utils/validate";

const DEFAULT_CACHE_NAME = "frimousse/data";

const validateEmojiData = $.object<EmojiData>({
  locale: $.string,
  emojis: $.naiveArray(
    $.object({
      emoji: $.string,
      category: $.number,
      label: $.string,
      version: $.number,
      tags: $.naiveArray($.string),
      aliases: $.optional($.naiveArray($.string)),
      countryFlag: $.optional($.boolean as $.Validator<true>),
      skins: $.optional(
        $.object({
          light: $.string,
          "medium-light": $.string,
          medium: $.string,
          "medium-dark": $.string,
          dark: $.string,
        }),
      ),
    }),
  ),
  categories: $.naiveArray(
    $.object({
      index: $.number,
      label: $.string,
    }),
  ),
  skinTones: $.object({
    light: $.string,
    "medium-light": $.string,
    medium: $.string,
    "medium-dark": $.string,
    dark: $.string,
  }),
});

export type CreateEmojiDataCacheOptions = {
  /**
   * The prefix for `localStorage` keys: `${name}/${locale}`.
   * Use a custom name to keep your data separate from the default resolver's.
   *
   * @default "frimousse/data"
   */
  name?: string;
};

/**
 * Caches emoji data and optional metadata in `localStorage`, keyed by locale.
 * Safe to create at module scope; storage is only accessed when used.
 * Storage failures are ignored, and metadata is read without validation.
 */
export function createEmojiDataCache<M = undefined>({
  name = DEFAULT_CACHE_NAME,
}: CreateEmojiDataCacheOptions = {}): EmojiDataCache<M> {
  const prefix = `${name}/`;
  const validateEntry = $.object<{ data: EmojiData; metadata: M }>({
    data: validateEmojiData,
    // The metadata is opaque, it's up to the resolver to make sense of it
    metadata: (metadata) => metadata as M,
  });

  return {
    get(locale) {
      try {
        return getStorage(localStorage, `${prefix}${locale}`, validateEntry);
      } catch {
        return null;
      }
    },

    set(locale, data, ...[metadata]) {
      try {
        setStorage(localStorage, `${prefix}${locale}`, { data, metadata });
      } catch {}
    },

    delete(locale) {
      try {
        localStorage.removeItem(`${prefix}${locale}`);
      } catch {}
    },

    clear() {
      try {
        for (const key of Object.keys(localStorage)) {
          if (key.startsWith(prefix)) {
            localStorage.removeItem(key);
          }
        }
      } catch {}
    },
  };
}
