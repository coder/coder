// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import type { RefObject } from "react";
import type {
  EmojiPickerData,
  EmojiPickerDataRow,
  EmojiPickerEmoji,
  EmojiPickerRootProps,
  Locale,
  SkinTone,
} from "./types";
import { createStore, createStoreContext } from "./utils/store";

const VIEWPORT_OVERSCAN = 2;

type Interaction = "keyboard" | "pointer" | "none";

export type EmojiPickerStore = {
  locale: Locale;
  columns: number;
  sticky: boolean;
  skinTone: SkinTone;
  onEmojiSelect: NonNullable<EmojiPickerRootProps["onEmojiSelect"]>;

  data: EmojiPickerData | null | undefined;
  search: string;
  interaction: Interaction;
  activeColumnIndex: number;
  activeRowIndex: number;

  rowHeight: number | null;
  categoryHeaderHeight: number | null;
  viewportWidth: number | null;
  viewportHeight: number | null;

  viewportStartCategoryIndex: number;
  viewportStartRowIndex: number;
  viewportEndRowIndex: number;

  rootRef: RefObject<HTMLDivElement> | null;
  searchRef: RefObject<HTMLInputElement> | null;
  viewportRef: RefObject<HTMLDivElement> | null;
  listRef: RefObject<HTMLDivElement> | null;

  updateViewportState: (changes?: Partial<EmojiPickerStore>) => void;

  onDataChange: (data: EmojiPickerData) => void;
  onSearchChange: (search: string) => void;
  onActiveEmojiChange: (
    interaction: Exclude<Interaction, "none">,
    activeColumnIndex: number,
    activeRowIndex: number,
  ) => void;
  onActiveEmojiReset: () => void;
  onRowHeightChange: (rowHeight: number) => void;
  onCategoryHeaderHeightChange: (categoryHeaderHeight: number) => void;
  onViewportSizeChange: (viewportWidth: number, viewportHeight: number) => void;
  onViewportScroll: (scrollY: number) => void;
};

export function createEmojiPickerStore(
  onEmojiSelect: NonNullable<EmojiPickerRootProps["onEmojiSelect"]>,
  initialLocale: Locale,
  initialColumns: number,
  initialSticky: boolean,
  initialSkinTone: SkinTone,
) {
  let viewportScrollY = 0;

  return createStore<EmojiPickerStore>((set, get) => ({
    locale: initialLocale,
    columns: initialColumns,
    sticky: initialSticky,
    skinTone: initialSkinTone,
    onEmojiSelect,

    data: null,
    search: "",
    interaction: "none",
    activeColumnIndex: 0,
    activeRowIndex: 0,

    rowHeight: null,
    categoryHeaderHeight: null,
    viewportWidth: null,
    viewportHeight: null,

    viewportCurrentCategoryIndex: null,
    viewportStartCategoryIndex: 0,
    viewportStartRowIndex: 0,
    viewportEndRowIndex: 0,

    rootRef: null,
    searchRef: null,
    viewportRef: null,
    listRef: null,

    updateViewportState: (partial?: Partial<EmojiPickerStore>) => {
      const state = get();

      const data = partial?.data ?? state.data;
      const categoryHeaderHeight =
        partial?.categoryHeaderHeight ?? state.categoryHeaderHeight;
      const rowHeight = partial?.rowHeight ?? state.rowHeight;
      const viewportHeight = partial?.viewportHeight ?? state.viewportHeight;

      if (
        !data ||
        data.rows.length === 0 ||
        !categoryHeaderHeight ||
        !rowHeight ||
        !viewportHeight
      ) {
        return set({
          ...partial,
          viewportStartCategoryIndex: 0,
          viewportStartRowIndex: 0,
          viewportEndRowIndex: 0,
        });
      }

      let previousCategoryHeadersHeight = 0;
      let categoryIndex = 0;

      for (const category of data.categories) {
        const categoryY =
          categoryIndex++ * categoryHeaderHeight +
          category.startRowIndex * rowHeight;

        if (categoryY < viewportScrollY) {
          previousCategoryHeadersHeight += categoryHeaderHeight;
        } else {
          break;
        }
      }

      const totalHeight =
        data.categories.length * categoryHeaderHeight +
        data.rows.length * rowHeight;

      const overscanStart = Math.floor((VIEWPORT_OVERSCAN * rowHeight) / 2);
      const overscanEnd = Math.ceil((VIEWPORT_OVERSCAN * rowHeight) / 2);

      // Adjust the scroll position to account for previous category headers
      const viewportStartY = Math.min(
        viewportScrollY - previousCategoryHeadersHeight - overscanStart,
        totalHeight - viewportHeight,
      );

      const viewportEndY = viewportStartY + viewportHeight + overscanEnd;

      const viewportStartRowIndex = Math.max(
        0,
        Math.floor(viewportStartY / rowHeight),
      );
      const viewportEndRowIndex = Math.min(
        data.rows.length - 1,
        Math.ceil(viewportEndY / rowHeight),
      );
      const viewportStartCategoryIndex =
        data.rows[viewportStartRowIndex]?.categoryIndex;

      if (viewportStartCategoryIndex === undefined && partial) {
        return set(partial);
      }

      return set({
        ...partial,
        viewportStartCategoryIndex,
        viewportStartRowIndex,
        viewportEndRowIndex,
      });
    },

    onDataChange: (data: EmojiPickerData) => {
      get().updateViewportState({
        data,

        // Reset active emoji when data changes
        activeColumnIndex: 0,
        activeRowIndex: 0,
      });
    },
    onSearchChange: (search: string) => {
      set({ search, interaction: search ? "keyboard" : "none" });
    },
    onActiveEmojiChange: (
      interaction: Exclude<Interaction, "none">,
      activeColumnIndex: number,
      activeRowIndex: number,
    ) => {
      set({
        interaction,
        activeColumnIndex,
        activeRowIndex,
      });

      if (interaction !== "keyboard") {
        return;
      }

      const {
        listRef,
        viewportRef,
        sticky,
        rowHeight,
        viewportHeight,
        categoryHeaderHeight,
      } = get();

      const list = listRef?.current;
      const viewport = viewportRef?.current;

      if (
        !list ||
        !viewport ||
        !rowHeight ||
        !categoryHeaderHeight ||
        !viewportHeight
      ) {
        return;
      }

      const rowIndex = activeRowIndex;

      if (rowIndex === 0) {
        viewport.scrollTo({
          top: 0,
          behavior: "instant",
        });
      }

      // LOCAL CHANGE: rows render a 1-based aria-rowindex (see listRowProps).
      const row = list.querySelector(`[aria-rowindex="${rowIndex + 1}"]`);

      if (!(row instanceof HTMLElement)) {
        return;
      }

      const rowY = row.offsetTop;
      const rowComputedStyle = getComputedStyle(row);
      const rowScrollMarginTop = Number.parseFloat(
        rowComputedStyle.scrollMarginTop,
      );
      const rowScrollMarginBottom = Number.parseFloat(
        rowComputedStyle.scrollMarginBottom,
      );

      let viewportStartY = viewportScrollY + rowScrollMarginTop;

      // Account for headers if they are sticky and if the row is in the upper half of the viewport
      if (sticky && rowY < viewportScrollY + viewportHeight / 2) {
        viewportStartY += categoryHeaderHeight;
      }

      const viewportEndY =
        viewportStartY + viewportHeight - rowScrollMarginBottom;

      if (rowY < viewportStartY || rowY + rowHeight > viewportEndY) {
        viewport.scrollTo({
          // Align to the viewport's top or bottom based on the row's position
          top: Math.max(
            rowY < viewportStartY + categoryHeaderHeight
              ? rowY -
                  Math.max(
                    sticky ? categoryHeaderHeight : 0,
                    rowScrollMarginTop,
                  )
              : rowY - viewportHeight + rowHeight + rowScrollMarginBottom,
            0,
          ),
          behavior: "instant",
        });
      }
    },
    onActiveEmojiReset: () => {
      set({
        interaction: "none",

        // Reset active emoji when interaction goes back to none
        activeColumnIndex: 0,
        activeRowIndex: 0,
      });
    },
    onRowHeightChange: (rowHeight: number) => {
      get().updateViewportState({ rowHeight });
    },
    onCategoryHeaderHeightChange: (categoryHeaderHeight: number) => {
      get().updateViewportState({ categoryHeaderHeight });
    },
    onViewportSizeChange: (viewportWidth: number, viewportHeight: number) => {
      get().updateViewportState({ viewportWidth, viewportHeight });
    },
    onViewportScroll: (scrollY: number) => {
      viewportScrollY = scrollY;

      get().updateViewportState();
    },
  }));
}

export const {
  useStore: useEmojiPickerStore,
  Provider: EmojiPickerStoreProvider,
} = createStoreContext<EmojiPickerStore>("EmojiPicker.Root is missing.");

export function $search(state: EmojiPickerStore) {
  return state.search;
}

export function $activeEmoji(
  state: EmojiPickerStore,
): EmojiPickerEmoji | undefined {
  if (state.interaction === "none") {
    return undefined;
  }

  const activeEmoji =
    state.data?.rows[state.activeRowIndex]?.emojis[state.activeColumnIndex];

  return activeEmoji;
}

export function $isEmpty(state: EmojiPickerStore) {
  return (
    state.data === undefined ||
    (typeof state.data?.count === "number" && state.data.count === 0)
  );
}

export function $isLoading(state: EmojiPickerStore) {
  return (
    state.data === null ||
    state.viewportHeight === null ||
    state.rowHeight === null ||
    state.categoryHeaderHeight === null
  );
}

export function $rowsCount(state: EmojiPickerStore) {
  return state.data?.rows.length;
}

export function $categoriesCount(state: EmojiPickerStore) {
  return state.data?.categories.length;
}

export function $categoriesRowsStartIndices(state: EmojiPickerStore) {
  return state.data?.categoriesStartRowIndices;
}

export function $skinTones(state: EmojiPickerStore) {
  return state.data?.skinTones;
}

export function sameEmojiPickerEmoji(
  a: EmojiPickerEmoji | undefined,
  b: EmojiPickerEmoji | undefined,
) {
  return a?.emoji === b?.emoji;
}

export function sameEmojiPickerRow(
  a: EmojiPickerDataRow | undefined,
  b: EmojiPickerDataRow | undefined,
) {
  if (a?.categoryIndex !== b?.categoryIndex) {
    return false;
  }

  if (a?.emojis.length !== b?.emojis.length) {
    return false;
  }

  return Boolean(
    a?.emojis.every((emoji, index) =>
      sameEmojiPickerEmoji(emoji, b?.emojis[index]),
    ),
  );
}
