// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

export * as EmojiPicker from "./components/emoji-picker";
// LOCAL CHANGE: exports Coder does not use are removed, along with the files
// only they reached (data/get-emoji-details.ts, hooks/use-emoji-details.ts).
export { useSkinTone } from "./hooks";
export type {
  EmojiData,
  EmojiPickerListCategoryHeaderProps,
  EmojiPickerListEmojiProps,
  EmojiPickerListRowProps,
} from "./types";
