// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import { SKIN_TONES } from "../constants";
import type {
  EmojibaseEmoji,
  EmojibaseEmojiWithGroup,
  EmojibaseLocale,
  EmojibaseMessagesDataset,
  EmojiData,
  EmojiDataEmoji,
  EmojiDataResolver,
  SkinTone,
} from "../types";
import { capitalize } from "../utils/capitalize";
import { isEmojiSupported } from "../utils/is-emoji-supported";
import { getStorage, setStorage } from "../utils/storage";
import * as $ from "../utils/validate";
import { createEmojiDataCache } from "./emoji-data-cache";
import {
  getCachedEmojiData,
  getEmojibaseUrl,
  setCachedEmojiData,
} from "./emoji-data-store";

const EMOJIBASE_EMOJIS_URL = (baseUrl: string, locale: EmojibaseLocale) =>
  `${baseUrl}/${locale}/data.json`;
const EMOJIBASE_MESSAGES_URL = (baseUrl: string, locale: EmojibaseLocale) =>
  `${baseUrl}/${locale}/messages.json`;

const EMOJIBASE_LOCALES = [
  "bn",
  "da",
  "de",
  "en-gb",
  "en",
  "es-mx",
  "es",
  "et",
  "fi",
  "fr",
  "hi",
  "hu",
  "it",
  "ja",
  "ko",
  "lt",
  "ms",
  "nb",
  "nl",
  "pl",
  "pt",
  "ru",
  "sv",
  "th",
  "uk",
  "vi",
  "zh-hant",
  "zh",
] satisfies EmojibaseLocale[];
const EMOJIBASE_DEFAULT_LOCALE: EmojibaseLocale = "en";

export const SESSION_METADATA_KEY = (baseUrl: string) =>
  `frimousse/metadata/${baseUrl}`;

// Keep the list in sync with Emojibase's supported locales
{
  type MissingLocales = Exclude<
    EmojibaseLocale,
    (typeof EMOJIBASE_LOCALES)[number]
  >;
  type AllLocalesPresent = MissingLocales extends never
    ? true
    : `Missing locales: ${MissingLocales}`;
  const _allLocalesPresent: AllLocalesPresent = true;
  _allLocalesPresent;
}

type EmojibaseMetadata = {
  emojisEtag: string | null;
  messagesEtag: string | null;
};

type EmojiSupport = {
  emojiVersion: number;
  countryFlags: boolean;
};

type SessionMetadata = Partial<EmojiSupport> & {
  revalidated: string[];
};

const pendingData = new Map<
  string,
  { promise: Promise<EmojiData>; controller: AbortController; callers: number }
>();

function createEmojibaseCache(baseUrl: string) {
  return createEmojiDataCache<EmojibaseMetadata>({
    name: `frimousse/data/${baseUrl}`,
  });
}

async function fetchEtag(url: string, signal?: AbortSignal) {
  try {
    const response = await fetch(url, { method: "HEAD", signal });

    return response.headers.get("etag");
  } catch (_) {
    return null;
  }
}

async function fetchEmojibaseData(
  baseUrl: string,
  locale: EmojibaseLocale,
  signal?: AbortSignal,
) {
  const [{ emojis, emojisEtag }, { messages, messagesEtag }] =
    await Promise.all([
      fetch(EMOJIBASE_EMOJIS_URL(baseUrl, locale), { signal }).then(
        async (response) => {
          return {
            emojis: (await response.json()) as EmojibaseEmoji[],
            emojisEtag: response.headers.get("etag"),
          };
        },
      ),
      fetch(EMOJIBASE_MESSAGES_URL(baseUrl, locale), { signal }).then(
        async (response) => {
          return {
            messages: (await response.json()) as EmojibaseMessagesDataset,
            messagesEtag: response.headers.get("etag"),
          };
        },
      ),
    ]);

  return {
    emojis,
    messages,
    emojisEtag,
    messagesEtag,
  };
}

async function fetchEmojibaseEtags(
  baseUrl: string,
  locale: EmojibaseLocale,
  signal?: AbortSignal,
) {
  const [emojisEtag, messagesEtag] = await Promise.all([
    fetchEtag(EMOJIBASE_EMOJIS_URL(baseUrl, locale), signal),
    fetchEtag(EMOJIBASE_MESSAGES_URL(baseUrl, locale), signal),
  ]);

  return {
    emojisEtag,
    messagesEtag,
  };
}

export function getEmojibaseSkinToneVariations(
  emoji: EmojibaseEmojiWithGroup,
): Record<Exclude<SkinTone, "none">, string> | undefined {
  if (!emoji.skins) {
    return;
  }

  const skinToneVariations = emoji.skins.filter(
    (emoji) => typeof emoji.tone === "number",
  );

  return skinToneVariations.reduce(
    (result, emoji) => {
      const skinTone = SKIN_TONES[emoji.tone as number]!;

      result[skinTone as Exclude<SkinTone, "none">] = emoji.emoji;

      return result;
    },
    {} as Record<Exclude<SkinTone, "none">, string>,
  );
}

async function fetchEmojiData(
  baseUrl: string,
  locale: EmojibaseLocale,
  signal?: AbortSignal,
): Promise<EmojiData> {
  const { emojis, emojisEtag, messages, messagesEtag } =
    await fetchEmojibaseData(baseUrl, locale, signal);
  signal?.throwIfAborted();
  const countryFlagsSubgroup = messages.subgroups.find(
    (subgroup) =>
      subgroup.key === "country-flag" || subgroup.key === "subdivision-flag",
  );

  // Filter out the component/modifier category and its emojis
  const filteredGroups = messages.groups.filter(
    (group) => group.key !== "component",
  );
  const filteredEmojis = emojis.filter((emoji) => {
    return "group" in emoji;
  }) as EmojibaseEmojiWithGroup[];

  const categories = filteredGroups.map((group) => ({
    index: group.order,
    label: capitalize(group.message),
  }));
  const skinTones = messages.skinTones.reduce(
    (skinTones, skinTone) => {
      skinTones[skinTone.key] = capitalize(skinTone.message);

      return skinTones;
    },
    {} as Record<SkinTone, string>,
  );

  const formattedEmojis = filteredEmojis.map((emoji) => {
    const aliases = emoji.skins
      ?.filter((skin) => Array.isArray(skin.tone))
      .map((skin) => skin.emoji);

    return {
      emoji: emoji.emoji,
      category: emoji.group,
      version: emoji.version,
      label: capitalize(emoji.label),
      tags: emoji.tags ?? [],
      countryFlag:
        (countryFlagsSubgroup &&
          emoji.subgroup === countryFlagsSubgroup.order) ||
        undefined,
      skins: getEmojibaseSkinToneVariations(emoji),
      aliases: aliases?.length ? aliases : undefined,
    } satisfies EmojiDataEmoji;
  });

  const emojiData: EmojiData = {
    locale,
    emojis: formattedEmojis,
    categories,
    skinTones,
  };

  createEmojibaseCache(baseUrl).set(locale, emojiData, {
    emojisEtag,
    messagesEtag,
  });

  return emojiData;
}

function getEmojiSupport(emojis: EmojiDataEmoji[]): EmojiSupport {
  const versionEmojis = new Map<number, string>();

  for (const emoji of emojis) {
    if (!versionEmojis.has(emoji.version)) {
      versionEmojis.set(emoji.version, emoji.emoji);
    }
  }

  const descendingVersions = [...versionEmojis.keys()].sort((a, b) => b - a);
  const highestVersion = descendingVersions[0] ?? 0;

  const supportsCountryFlags = isEmojiSupported("🇪🇺");

  for (const version of descendingVersions) {
    const emoji = versionEmojis.get(version)!;

    if (isEmojiSupported(emoji)) {
      return {
        emojiVersion: version,
        countryFlags: supportsCountryFlags,
      };
    }
  }

  return {
    emojiVersion: highestVersion,
    countryFlags: supportsCountryFlags,
  };
}

const validateSessionMetadata = $.object<SessionMetadata>({
  emojiVersion: $.optional($.number),
  countryFlags: $.optional($.boolean),
  revalidated: $.naiveArray($.string),
});

function getSessionMetadata(baseUrl: string) {
  try {
    return getStorage(
      sessionStorage,
      SESSION_METADATA_KEY(baseUrl),
      validateSessionMetadata,
    );
  } catch {
    return null;
  }
}

function setSessionMetadata(baseUrl: string, metadata: SessionMetadata) {
  try {
    setStorage(sessionStorage, SESSION_METADATA_KEY(baseUrl), metadata);
  } catch {}
}

export function loadEmojiData(
  baseUrl: string,
  locale: EmojibaseLocale,
  signal?: AbortSignal,
): Promise<EmojiData> {
  if (signal?.aborted) {
    return Promise.reject(signal.reason);
  }

  const key = EMOJIBASE_EMOJIS_URL(baseUrl, locale);
  const data = getCachedEmojiData(locale, { emojibaseUrl: baseUrl });

  if (data) {
    return Promise.resolve(data);
  }

  let pending = pendingData.get(key);

  if (!pending || pending.controller.signal.aborted) {
    const controller = new AbortController();
    const promise = readEmojiData(baseUrl, locale, controller.signal)
      .then((data) => {
        controller.signal.throwIfAborted();
        setCachedEmojiData(locale, { emojibaseUrl: baseUrl }, data);
        return data;
      })
      .finally(() => {
        if (pendingData.get(key)?.promise === promise) {
          pendingData.delete(key);
        }
      });

    pending = { promise, controller, callers: 0 };
    pendingData.set(key, pending);
  }

  const request = pending;
  request.callers++;
  let onAbort: () => void;

  return new Promise<EmojiData>((resolve, reject) => {
    onAbort = () => reject(signal?.reason);
    signal?.addEventListener("abort", onAbort, { once: true });
    request.promise.then(resolve, reject);
  }).finally(() => {
    signal?.removeEventListener("abort", onAbort);
    request.callers--;

    // A caller must not cancel a load that another picker or lookup still needs.
    if (request.callers === 0 && pendingData.get(key) === request) {
      request.controller.abort();
    }
  });
}

async function readEmojiData(
  baseUrl: string,
  locale: EmojibaseLocale,
  signal: AbortSignal,
): Promise<EmojiData> {
  const sessionMetadata = getSessionMetadata(baseUrl);
  const cached = createEmojibaseCache(baseUrl).get(locale);

  let data: EmojiData;

  if (!cached) {
    // No cached data
    data = await fetchEmojiData(baseUrl, locale, signal);
  } else if (sessionMetadata?.revalidated.includes(locale)) {
    // Check ETags only once per locale per session
    data = cached.data;
  } else {
    // Check ETags to see if the cached data is up-to-date,
    // but if that fails, the possibly-stale cached data is used
    try {
      const { emojisEtag, messagesEtag } = await fetchEmojibaseEtags(
        baseUrl,
        locale,
        signal,
      );
      signal.throwIfAborted();

      data =
        !emojisEtag ||
        !messagesEtag ||
        emojisEtag !== cached.metadata?.emojisEtag ||
        messagesEtag !== cached.metadata?.messagesEtag
          ? await fetchEmojiData(baseUrl, locale, signal)
          : cached.data;
    } catch {
      signal.throwIfAborted();
      data = cached.data;
    }
  }

  signal.throwIfAborted();
  const metadata = getSessionMetadata(baseUrl);
  const revalidated = metadata?.revalidated ?? [];

  setSessionMetadata(baseUrl, {
    ...metadata,
    revalidated: revalidated.includes(locale)
      ? revalidated
      : [...revalidated, locale],
  });

  return data;
}

/**
 * Fetches and caches Emojibase data, filtering out unsupported emojis.
 * Use it for locales a custom resolver doesn't handle.
 */
export const defaultEmojiDataResolver: EmojiDataResolver = async (
  locale,
  { emojiVersion, emojibaseUrl, signal } = {},
) => {
  const emojibaseLocale = validateLocale(locale);
  const baseUrl = getEmojibaseUrl({ emojiVersion, emojibaseUrl });
  const data = await loadEmojiData(baseUrl, emojibaseLocale, signal);
  signal?.throwIfAborted();
  const sessionMetadata = getSessionMetadata(baseUrl);
  const support: EmojiSupport =
    sessionMetadata?.emojiVersion !== undefined &&
    sessionMetadata.countryFlags !== undefined
      ? {
          emojiVersion: sessionMetadata.emojiVersion,
          countryFlags: sessionMetadata.countryFlags,
        }
      : getEmojiSupport(data.emojis);

  setSessionMetadata(baseUrl, {
    ...support,
    revalidated: sessionMetadata?.revalidated ?? [emojibaseLocale],
  });

  // Filter out unsupported emojis
  const filteredEmojis = data.emojis.filter((emoji) => {
    const isSupportedVersion =
      emoji.version <= (emojiVersion ?? support.emojiVersion);

    return emoji.countryFlag
      ? isSupportedVersion && support.countryFlags
      : isSupportedVersion;
  });

  return {
    locale: emojibaseLocale,
    emojis: filteredEmojis,
    categories: data.categories,
    skinTones: data.skinTones,
  };
};

export function validateLocale(locale: string): EmojibaseLocale {
  const emojibaseLocale = EMOJIBASE_LOCALES.find(
    (supportedLocale) => supportedLocale === locale,
  );

  if (!emojibaseLocale) {
    console.warn(
      `Locale "${locale}" is not supported, using "${EMOJIBASE_DEFAULT_LOCALE}" instead.`,
    );

    return EMOJIBASE_DEFAULT_LOCALE;
  }

  return emojibaseLocale;
}

export function validateSkinTone(skinTone: string): SkinTone {
  if (!SKIN_TONES.includes(skinTone as SkinTone)) {
    console.warn(`Skin tone "${skinTone}" is not valid, using "none" instead.`);

    return "none";
  }

  return skinTone as SkinTone;
}
