// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import { SKIN_TONES } from "../constants";
import type { SkinTone, SkinToneVariation } from "../types";

const ZWJ = "\u200D";
const EMOJI_MODIFIER_BASE = /\p{Emoji_Modifier_Base}/u;
const ENDING_VARIATION_SELECTOR = /\uFE0F$/;
const SKIN_TONE_MODIFIERS =
  /\u{1F3FB}|\u{1F3FC}|\u{1F3FD}|\u{1F3FE}|\u{1F3FF}/gu;

const skinToneModifiers: Record<Exclude<SkinTone, "none">, string> = {
  light: "\u{1F3FB}",
  "medium-light": "\u{1F3FC}",
  medium: "\u{1F3FD}",
  "medium-dark": "\u{1F3FE}",
  dark: "\u{1F3FF}",
};

export function getSkinToneVariation(emoji: string, skinTone: SkinTone) {
  // The emoji does not support skin tones
  if (!emoji.split(ZWJ).some((segment) => EMOJI_MODIFIER_BASE.test(segment))) {
    return emoji;
  }

  const baseEmoji = emoji
    .split(ZWJ)
    .map((segment) => segment.replace(SKIN_TONE_MODIFIERS, ""))
    .join(ZWJ);

  if (skinTone === "none") {
    return baseEmoji;
  }

  return baseEmoji
    .split(ZWJ)
    .map((segment, _, segments) => {
      const isZwjSequence = segments.length > 1;

      if (
        !EMOJI_MODIFIER_BASE.test(segment) ||
        // The 🤝 emoji should not be toned within a ZWJ sequence (e.g. 🧑‍🤝‍🧑)
        (isZwjSequence && segment === "🤝")
      ) {
        return segment;
      }

      return (
        segment.replace(ENDING_VARIATION_SELECTOR, "") +
        skinToneModifiers[skinTone]
      );
    })
    .join(ZWJ);
}

export function getSkinToneVariations(emoji: string): SkinToneVariation[] {
  return SKIN_TONES.map((skinTone) => ({
    skinTone,
    emoji: getSkinToneVariation(emoji, skinTone),
  }));
}
