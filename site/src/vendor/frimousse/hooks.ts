// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import { useCallback, useDeferredValue, useMemo } from "react";
import type * as EmojiPicker from "./components/emoji-picker";
import {
  $activeEmoji,
  sameEmojiPickerEmoji,
  useEmojiPickerStore,
} from "./store";
import type { Emoji, SkinTone, SkinToneVariation } from "./types";
import { getSkinToneVariations } from "./utils/get-skin-tone-variations";
import { useSelector, useSelectorKey } from "./utils/store";

/**
 * Returns the currently active emoji (either hovered or selected
 * via keyboard navigation).
 *
 * @example
 * ```tsx
 * const activeEmoji = useActiveEmoji();
 * ```
 *
 * It can be used to build a preview area next to the list.
 *
 * @example
 * ```tsx
 * const activeEmoji = useActiveEmoji();
 *
 * <div>
 *   {activeEmoji ? (
 *     <span>{activeEmoji.emoji} {activeEmoji.label}</span>
 *   ) : (
 *     <span>Select an emoji…</span>
 *   )}
 * </div>
 * ```
 *
 * @see
 * If you prefer to use a component rather than a hook,
 * {@link EmojiPicker.ActiveEmoji|`<EmojiPicker.ActiveEmoji />`} is also available.
 */
export function useActiveEmoji(): Emoji | undefined {
  const store = useEmojiPickerStore();
  const activeEmoji = useSelector(store, $activeEmoji, sameEmojiPickerEmoji);

  return useDeferredValue(activeEmoji);
}

/**
 * Returns the current skin tone and a function to change it.
 *
 * @example
 * ```tsx
 * const [skinTone, setSkinTone] = useSkinTone();
 * ```
 *
 * It can be used to build a custom skin tone selector: pass an emoji
 * you want to use as visual (by default, ✋) and it will return its skin tone
 * variations.
 *
 * @example
 * ```tsx
 * const [skinTone, setSkinTone, skinToneVariations] = useSkinTone("👋");
 *
 * // (👋) (👋🏻) (👋🏼) (👋🏽) (👋🏾) (👋🏿)
 * skinToneVariations.map(({ skinTone, emoji }) => (
 *   <button key={skinTone} onClick={() => setSkinTone(skinTone)}>
 *     {emoji}
 *   </button>
 * ));
 * ```
 *
 * @see
 * If you prefer to use a component rather than a hook,
 * {@link EmojiPicker.SkinTone|`<EmojiPicker.SkinTone />`} is also available.
 *
 * @see
 * An already-built skin tone selector is also available,
 * {@link EmojiPicker.SkinToneSelector|`<EmojiPicker.SkinToneSelector />`}.
 */
export function useSkinTone(
  emoji = "✋",
): [SkinTone, (skinTone: SkinTone) => void, SkinToneVariation[]] {
  const store = useEmojiPickerStore();
  const skinTone = useSelectorKey(store, "skinTone");
  const skinToneVariations = useMemo(
    () => getSkinToneVariations(emoji),
    [emoji],
  );

  const setSkinTone = useCallback((skinTone: SkinTone) => {
    store.set({ skinTone });
  }, []);

  return [skinTone, setSkinTone, skinToneVariations];
}
