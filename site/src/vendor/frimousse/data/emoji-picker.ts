// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import type {
  Emoji,
  EmojiData,
  EmojiDataEmoji,
  EmojiPickerData,
  EmojiPickerDataCategory,
  EmojiPickerDataRow,
  EmojiPickerEmoji,
  SkinTone,
} from "../types";
import { chunk } from "../utils/chunk";

export function searchEmojis(emojis: EmojiDataEmoji[], search?: string) {
  if (!search) {
    return emojis;
  }

  const searchText = search.toLowerCase().trim();
  const scores = new WeakMap<Emoji, number>();

  return emojis
    .filter((emoji) => {
      let score = 0;

      if (emoji.label.toLowerCase().includes(searchText)) {
        score += 10;
      }

      for (const tag of emoji.tags) {
        if (tag.toLowerCase().includes(searchText)) {
          score += 1;
        }
      }

      if (score > 0) {
        scores.set(emoji, score);

        return true;
      }

      return false;
    })
    .sort((a, b) => (scores.get(b) ?? 0) - (scores.get(a) ?? 0));
}

export function getEmojiPickerData(
  data: EmojiData,
  columns: number,
  skinTone: SkinTone | undefined,
  search: string,
): EmojiPickerData {
  const emojis = searchEmojis(data.emojis, search);
  const rows: EmojiPickerDataRow[] = [];
  const categories: EmojiPickerDataCategory[] = [];
  const categoriesStartRowIndices: number[] = [];
  const emojisByCategory: Record<number, EmojiPickerEmoji[]> = {};
  let categoryIndex = 0;
  let startRowIndex = 0;

  for (const emoji of emojis) {
    if (!emojisByCategory[emoji.category]) {
      emojisByCategory[emoji.category] = [];
    }

    emojisByCategory[emoji.category]!.push({
      emoji:
        skinTone && skinTone !== "none" && emoji.skins
          ? emoji.skins[skinTone]
          : emoji.emoji,
      label: emoji.label,
    });
  }

  for (const category of data.categories) {
    const categoryEmojis = emojisByCategory[category.index];

    if (!categoryEmojis || categoryEmojis.length === 0) {
      continue;
    }

    const categoryRows = chunk(Array.from(categoryEmojis), columns).map(
      (emojis) => ({
        categoryIndex,
        emojis,
      }),
    );

    rows.push(...categoryRows);
    categories.push({
      label: category.label,
      rowsCount: categoryRows.length,
      startRowIndex,
    });

    categoriesStartRowIndices.push(startRowIndex);

    categoryIndex++;
    startRowIndex += categoryRows.length;
  }

  return {
    count: emojis.length,
    categories,
    categoriesStartRowIndices,
    rows,
    skinTones: data.skinTones,
  };
}
