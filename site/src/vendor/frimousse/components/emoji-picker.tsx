// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import {
  type ComponentProps,
  type CSSProperties,
  Fragment,
  forwardRef,
  memo,
  type ChangeEvent as ReactChangeEvent,
  type FocusEvent as ReactFocusEvent,
  type MouseEvent as ReactMouseEvent,
  type ReactNode,
  type SyntheticEvent as ReactSyntheticEvent,
  type UIEvent as ReactUIEvent,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import { EMOJI_FONT_FAMILY } from "../constants";
import { defaultEmojiDataResolver, validateSkinTone } from "../data/emoji";
import { resolveCustomEmojiData } from "../data/emoji-data-store";
import { getEmojiPickerData } from "../data/emoji-picker";
import { useActiveEmoji, useSkinTone } from "../hooks";
import {
  $activeEmoji,
  $categoriesCount,
  $categoriesRowsStartIndices,
  $isEmpty,
  $isLoading,
  $rowsCount,
  $search,
  $skinTones,
  createEmojiPickerStore,
  type EmojiPickerStore,
  EmojiPickerStoreProvider,
  sameEmojiPickerRow,
  useEmojiPickerStore,
} from "../store";
import type {
  EmojiData,
  EmojiDataResolverOptions,
  EmojiPickerActiveEmojiProps,
  EmojiPickerCategory,
  EmojiPickerDataCategory,
  EmojiPickerEmoji,
  EmojiPickerEmptyProps,
  EmojiPickerListCategoryHeaderProps,
  EmojiPickerListComponents,
  EmojiPickerListEmojiProps,
  EmojiPickerListProps,
  EmojiPickerListRowProps,
  EmojiPickerLoadingProps,
  EmojiPickerRootProps,
  EmojiPickerSearchProps,
  EmojiPickerSkinToneProps,
  EmojiPickerSkinToneSelectorProps,
  EmojiPickerViewportProps,
  Locale,
  WithAttributes,
} from "../types";
import { shallow } from "../utils/compare";
import { noop } from "../utils/noop";
import { requestIdleCallback } from "../utils/request-idle-callback";
import { useCreateStore, useSelector, useSelectorKey } from "../utils/store";
import { useLayoutEffect } from "../utils/use-layout-effect";
import { useStableCallback } from "../utils/use-stable-callback";

function EmojiPickerDataHandler({
  emojiVersion,
  emojibaseUrl,
  resolveEmojiData = defaultEmojiDataResolver,
}: Pick<
  EmojiPickerRootProps,
  "emojiVersion" | "emojibaseUrl" | "resolveEmojiData"
>) {
  const [emojiData, setEmojiData] = useState<EmojiData | undefined>(undefined);
  const store = useEmojiPickerStore();
  const locale = useSelectorKey(store, "locale");
  const columns = useSelectorKey(store, "columns");
  const skinTone = useSelectorKey(store, "skinTone");
  const search = useSelectorKey(store, "search");
  // Inline resolvers shouldn't reload data on every render.
  const stableResolveEmojiData = useStableCallback(
    (locale: Locale, options: EmojiDataResolverOptions) =>
      resolveEmojiData === defaultEmojiDataResolver
        ? defaultEmojiDataResolver(locale, options)
        : resolveCustomEmojiData(resolveEmojiData, locale, options),
  );

  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;

    Promise.resolve()
      .then(() =>
        stableResolveEmojiData(locale, { emojiVersion, emojibaseUrl, signal }),
      )
      .then((data) => {
        if (!signal.aborted) {
          setEmojiData(data);
        }
      })
      .catch((error) => {
        if (!signal.aborted) {
          console.error(error);
        }
      });

    return () => {
      controller.abort();
    };
  }, [emojiVersion, emojibaseUrl, locale, stableResolveEmojiData]);

  useEffect(() => {
    if (!emojiData) {
      return;
    }

    return requestIdleCallback(
      () => {
        store
          .get()
          .onDataChange(
            getEmojiPickerData(emojiData, columns, skinTone, search),
          );
      },
      { timeout: 100 },
    );
  }, [emojiData, columns, skinTone, search]);

  return null;
}

/**
 * Surrounds all the emoji picker parts.
 *
 * @example
 * ```tsx
 * <EmojiPicker.Root onEmojiSelect={({ emoji }) => console.log(emoji)}>
 *   <EmojiPicker.Search />
 *   <EmojiPicker.Viewport>
 *     <EmojiPicker.List />
 *   </EmojiPicker.Viewport>
 * </EmojiPicker.Root>
 * ```
 *
 * Options affecting the entire emoji picker are available on this
 * component as props.
 *
 * @example
 * ```tsx
 * <EmojiPicker.Root locale="fr" columns={10} skinTone="medium">
 *   {\/* ... *\/}
 * </EmojiPicker.Root>
 * ```
 */
const EmojiPickerRoot = forwardRef<HTMLDivElement, EmojiPickerRootProps>(
  (
    {
      locale = "en",
      columns = 9,
      skinTone = "none",
      onEmojiSelect = noop,
      emojiVersion,
      emojibaseUrl,
      resolveEmojiData,
      onFocusCapture,
      onBlurCapture,
      children,
      style,
      sticky = true,
      ...props
    },
    forwardedRef,
  ) => {
    const stableOnEmojiSelect = useStableCallback(onEmojiSelect);
    const store = useCreateStore(() =>
      createEmojiPickerStore(
        stableOnEmojiSelect,
        locale,
        columns,
        sticky,
        validateSkinTone(skinTone),
      ),
    );
    const [isFocusedWithin, setFocusedWithin] = useState(false);
    const ref = useRef<HTMLDivElement>(null!);
    const callbackRef = useCallback((element: HTMLDivElement | null) => {
      if (element) {
        ref.current = element;
        store.set({ rootRef: ref });
      }
    }, []);

    useLayoutEffect(() => {
      store.set({ locale });
    }, [locale]);

    useLayoutEffect(() => {
      store.set({ columns });
    }, [columns]);

    useLayoutEffect(() => {
      store.set({ sticky });
    }, [sticky]);

    useLayoutEffect(() => {
      store.set({ skinTone: validateSkinTone(skinTone) });
    }, [skinTone]);

    const handleFocusCapture = useCallback(
      (event: ReactFocusEvent<HTMLDivElement>) => {
        onFocusCapture?.(event);

        const { searchRef, viewportRef } = store.get();

        const isSearch =
          event.target === searchRef?.current ||
          event.target.hasAttribute("frimousse-search");

        const isViewport =
          event.target === viewportRef?.current ||
          event.target.hasAttribute("frimousse-viewport");

        if (!event.isDefaultPrevented()) {
          setFocusedWithin(isSearch || isViewport);

          if (!event.isDefaultPrevented()) {
            setFocusedWithin(isSearch || isViewport);

            if (isViewport) {
              store.get().onActiveEmojiChange("keyboard", 0, 0);
            } else if (isSearch && store.get().search === "") {
              store.set({ interaction: "none" });
            }
          }
        }
      },
      [onFocusCapture],
    );

    const handleBlurCapture = useCallback(
      (event: ReactFocusEvent<HTMLDivElement>) => {
        onBlurCapture?.(event);

        if (
          !event.isDefaultPrevented() &&
          !event.currentTarget.contains(event.relatedTarget)
        ) {
          setFocusedWithin(false);
        }
      },
      [onBlurCapture],
    );

    useLayoutEffect(() => {
      if (!isFocusedWithin) {
        store.get().onActiveEmojiReset();
      }
    }, [isFocusedWithin]);

    useImperativeHandle(forwardedRef, () => ref.current);

    useEffect(() => {
      if (!isFocusedWithin) {
        return;
      }

      function handleKeyDown(event: KeyboardEvent) {
        if (
          event.defaultPrevented ||
          (!event.key.startsWith("Arrow") && event.key !== "Enter")
        ) {
          return;
        }

        const {
          data,
          onEmojiSelect,
          onActiveEmojiChange,
          interaction,
          activeColumnIndex,
          activeRowIndex,
        } = store.get();

        // Select the active emoji with enter if it exists
        if (event.key === "Enter") {
          const activeEmoji = $activeEmoji(store.get());

          if (activeEmoji) {
            event.preventDefault();

            onEmojiSelect(activeEmoji);
          }
        }

        // Move the active emoji with arrow keys
        if (event.key.startsWith("Arrow")) {
          let columnIndex = activeColumnIndex;
          let rowIndex = activeRowIndex;

          event.preventDefault();

          if (interaction !== "none") {
            if (data?.rows && data.rows.length > 0) {
              switch (event.key) {
                case "ArrowLeft": {
                  if (columnIndex === 0) {
                    const previousRowIndex = rowIndex - 1;
                    const previousRow = data.rows[previousRowIndex];

                    // If first column, move to last column of previous row (if available)
                    if (previousRow) {
                      rowIndex = previousRowIndex;
                      columnIndex = previousRow.emojis.length - 1;
                    }
                  } else {
                    // Otherwise, move to previous column
                    columnIndex -= 1;
                  }

                  break;
                }

                case "ArrowRight": {
                  if (columnIndex === data.rows[rowIndex]!.emojis.length - 1) {
                    const nextRowIndex = rowIndex + 1;
                    const nextRow = data.rows[nextRowIndex];

                    // If last column, move to first column of next row (if available)
                    if (nextRow) {
                      rowIndex = nextRowIndex;
                      columnIndex = 0;
                    }
                  } else {
                    // Otherwise, move to next column
                    columnIndex += 1;
                  }

                  break;
                }

                case "ArrowUp": {
                  const previousRow = data.rows[rowIndex - 1];

                  // If not first row, move to previous row
                  if (previousRow) {
                    rowIndex -= 1;

                    // If previous row doesn't have the same column, move to last column of previous row
                    if (!previousRow.emojis[columnIndex]) {
                      columnIndex = previousRow.emojis.length - 1;
                    }
                  }

                  break;
                }

                case "ArrowDown": {
                  const nextRow = data.rows[rowIndex + 1];

                  // If not last row, move to next row
                  if (nextRow) {
                    rowIndex += 1;

                    // If next row doesn't have the same column, move to last column of next row
                    if (!nextRow.emojis[columnIndex]) {
                      columnIndex = nextRow.emojis.length - 1;
                    }
                  }

                  break;
                }
              }
            }

            onActiveEmojiChange("keyboard", columnIndex, rowIndex);
          } else {
            onActiveEmojiChange("keyboard", 0, 0);
          }
        }
      }

      document.addEventListener("keydown", handleKeyDown);

      return () => {
        document.removeEventListener("keydown", handleKeyDown);
      };
    }, [isFocusedWithin]);

    useLayoutEffect(() => {
      let previousViewportWidth: EmojiPickerStore["viewportWidth"] = null;
      let previousViewportHeight: EmojiPickerStore["viewportHeight"] = null;
      let previousRowHeight: EmojiPickerStore["rowHeight"] = null;
      let previousCategoryHeaderHeight: EmojiPickerStore["categoryHeaderHeight"] =
        null;

      const unsubscribe = store.subscribe((state) => {
        /* v8 ignore next 3 */
        if (!ref.current) {
          return;
        }

        if (previousViewportWidth !== state.viewportWidth) {
          previousViewportWidth = state.viewportWidth;

          ref.current.style.setProperty(
            "--frimousse-viewport-width",
            `${state.viewportWidth}px`,
          );
        }

        if (previousViewportHeight !== state.viewportHeight) {
          previousViewportHeight = state.viewportHeight;

          ref.current.style.setProperty(
            "--frimousse-viewport-height",
            `${state.viewportHeight}px`,
          );
        }

        if (previousRowHeight !== state.rowHeight) {
          previousRowHeight = state.rowHeight;

          ref.current.style.setProperty(
            "--frimousse-row-height",
            `${state.rowHeight}px`,
          );
        }

        if (previousCategoryHeaderHeight !== state.categoryHeaderHeight) {
          previousCategoryHeaderHeight = state.categoryHeaderHeight;

          ref.current.style.setProperty(
            "--frimousse-category-header-height",
            `${state.categoryHeaderHeight}px`,
          );
        }
      });

      const { viewportWidth, viewportHeight, rowHeight, categoryHeaderHeight } =
        store.get();

      if (viewportWidth) {
        ref.current.style.setProperty(
          "--frimousse-viewport-width",
          `${viewportWidth}px`,
        );
      }

      if (viewportHeight) {
        ref.current.style.setProperty(
          "--frimousse-viewport-height",
          `${viewportHeight}px`,
        );
      }

      if (rowHeight) {
        ref.current.style.setProperty(
          "--frimousse-row-height",
          `${rowHeight}px`,
        );
      }

      if (categoryHeaderHeight) {
        ref.current.style.setProperty(
          "--frimousse-category-header-height",
          `${categoryHeaderHeight}px`,
        );
      }

      return unsubscribe;
    }, []);

    return (
      <div
        data-focused={isFocusedWithin ? "" : undefined}
        frimousse-root=""
        onBlurCapture={handleBlurCapture}
        onFocusCapture={handleFocusCapture}
        {...props}
        ref={callbackRef}
        style={
          {
            "--frimousse-emoji-font": EMOJI_FONT_FAMILY,
            ...style,
          } as CSSProperties
        }
      >
        <EmojiPickerStoreProvider store={store}>
          <EmojiPickerDataHandler
            emojibaseUrl={emojibaseUrl}
            emojiVersion={emojiVersion}
            resolveEmojiData={resolveEmojiData}
          />
          {children}
        </EmojiPickerStoreProvider>
      </div>
    );
  },
);

/**
 * A search input to filter the list of emojis.
 *
 * @example
 * ```tsx
 * <EmojiPicker.Root>
 *   <EmojiPicker.Search />
 *   <EmojiPicker.Viewport>
 *     <EmojiPicker.List />
 *   </EmojiPicker.Viewport>
 * </EmojiPicker.Root>
 * ```
 *
 * It can be controlled or uncontrolled.
 *
 * @example
 * ```tsx
 * const [search, setSearch] = useState("");
 *
 * return (
 *   <EmojiPicker.Root>
 *     <EmojiPicker.Search
 *       value={search}
 *       onChange={(event) => setSearch(event.target.value)}
 *     />
 *     {\/* ... *\/}
 *   </EmojiPicker.Root>
 * );
 * ```
 */
const EmojiPickerSearch = forwardRef<HTMLInputElement, EmojiPickerSearchProps>(
  ({ value, defaultValue, onChange, ...props }, forwardedRef) => {
    const store = useEmojiPickerStore();
    const ref = useRef<HTMLInputElement>(null!);
    const callbackRef = useCallback((element: HTMLInputElement | null) => {
      if (element) {
        ref.current = element;
        store.set({ searchRef: ref });
      }
    }, []);
    const isControlled = typeof value === "string";
    const wasControlled = useRef(isControlled);

    useEffect(() => {
      if (
        process.env.NODE_ENV !== "production" &&
        wasControlled.current !== isControlled
      ) {
        console.warn(
          `EmojiPicker.Search is changing from ${
            wasControlled ? "controlled" : "uncontrolled"
          } to ${isControlled ? "controlled" : "uncontrolled"}.`,
        );
      }

      wasControlled.current = isControlled;
    }, [isControlled]);

    // Initialize search with a controlled or uncontrolled value
    useLayoutEffect(() => {
      store.set({
        search:
          typeof value === "string"
            ? value
            : typeof defaultValue === "string"
              ? defaultValue
              : "",
      });
    }, []);

    // Handle controlled value changes
    useLayoutEffect(() => {
      if (typeof value === "string") {
        store.get().onSearchChange(value);
      }
    }, [value]);

    const handleChange = useCallback(
      (event: ReactChangeEvent<HTMLInputElement>) => {
        onChange?.(event);

        if (!event.isDefaultPrevented()) {
          store.get().onSearchChange(event.target.value);
        }
      },
      [onChange],
    );

    useImperativeHandle(forwardedRef, () => ref.current);

    return (
      <input
        autoCapitalize="off"
        autoComplete="off"
        autoCorrect="off"
        enterKeyHint="done"
        frimousse-search=""
        placeholder="Search…"
        spellCheck={false}
        type="search"
        {...props}
        defaultValue={defaultValue}
        onChange={handleChange}
        ref={callbackRef}
        value={value}
      />
    );
  },
);

const ActiveEmojiAnnouncer = memo(() => {
  const activeEmoji = useActiveEmoji();

  if (!activeEmoji) {
    return null;
  }

  return (
    <div
      aria-live="polite"
      style={{
        border: 0,
        clip: "rect(0, 0, 0, 0)",
        height: 1,
        margin: -1,
        overflow: "hidden",
        padding: 0,
        position: "absolute",
        whiteSpace: "nowrap",
        width: 1,
        wordWrap: "normal",
      }}
    >
      {activeEmoji.label}
    </div>
  );
});

/**
 * The scrolling container of the emoji picker.
 *
 * @example
 * ```tsx
 * <EmojiPicker.Root>
 *   <EmojiPicker.Search />
 *   <EmojiPicker.Viewport>
 *     <EmojiPicker.Loading>Loading…</EmojiPicker.Loading>
 *     <EmojiPicker.Empty>No emoji found.</EmojiPicker.Empty>
 *     <EmojiPicker.List />
 *   </EmojiPicker.Viewport>
 * </EmojiPicker.Root>
 * ```
 */
const EmojiPickerViewport = forwardRef<
  HTMLDivElement,
  EmojiPickerViewportProps
>(({ children, onScroll, onKeyDown, style, ...props }, forwardedRef) => {
  const store = useEmojiPickerStore();
  const ref = useRef<HTMLDivElement>(null!);
  const callbackRef = useCallback((element: HTMLDivElement | null) => {
    if (element) {
      ref.current = element;
      store.set({ viewportRef: ref });
    }
  }, []);
  const rowsCount = useSelector(store, $rowsCount);
  const categoriesCount = useSelector(store, $categoriesCount);

  const handleScroll = useCallback(
    (event: ReactUIEvent<HTMLDivElement>) => {
      onScroll?.(event);

      store.get().onViewportScroll(event.currentTarget.scrollTop);
    },
    [onScroll],
  );

  useLayoutEffect(() => {
    /* v8 ignore next 3 */
    if (!ref.current) {
      return;
    }

    const resizeObserver = new ResizeObserver(([entry]) => {
      const width = entry?.borderBoxSize[0]?.inlineSize ?? 0;
      const height = entry?.borderBoxSize[0]?.blockSize ?? 0;

      const { onViewportSizeChange, viewportHeight, viewportWidth } =
        store.get();

      if (viewportHeight !== height || viewportWidth !== width) {
        onViewportSizeChange(width, height);
      }
    });

    resizeObserver.observe(ref.current);

    store
      .get()
      .onViewportSizeChange(ref.current.offsetWidth, ref.current.clientHeight);

    return () => {
      resizeObserver.disconnect();
    };
  }, []);

  useImperativeHandle(forwardedRef, () => ref.current);

  return (
    <div
      frimousse-viewport=""
      {...props}
      onScroll={handleScroll}
      ref={callbackRef}
      style={{
        position: "relative",
        boxSizing: "border-box",
        contain: "layout paint",
        containIntrinsicSize:
          typeof rowsCount === "number" && typeof categoriesCount === "number"
            ? `var(--frimousse-viewport-width, auto) calc(${rowsCount} * var(--frimousse-row-height) + ${categoriesCount} * var(--frimousse-category-header-height))`
            : undefined,
        overflowY: "auto",
        overscrollBehavior: "contain",
        scrollbarGutter: "stable",
        willChange: "scroll-position",
        ...style,
      }}
    >
      <ActiveEmojiAnnouncer />
      {children}
    </div>
  );
});

function listEmojiProps(
  emoji: EmojiPickerEmoji,
  columnIndex: number,
  isActive: boolean,
): WithAttributes<EmojiPickerListEmojiProps> {
  return {
    emoji: { ...emoji, isActive },
    role: "gridcell",
    // LOCAL CHANGE: ARIA grid indexes start at 1.
    "aria-colindex": columnIndex + 1,
    "aria-selected": isActive || undefined,
    "aria-label": emoji.label,
    "data-active": isActive ? "" : undefined,
    "frimousse-emoji": "",
    style: {
      fontFamily: "var(--frimousse-emoji-font)",
    },
    tabIndex: -1,
  };
}

function listRowProps(
  rowIndex: number,
  sizer = false,
): WithAttributes<EmojiPickerListRowProps> {
  return {
    role: !sizer ? "row" : undefined,
    // LOCAL CHANGE: ARIA grid indexes start at 1; store.ts looks rows up by
    // this value.
    "aria-rowindex": !sizer ? rowIndex + 1 : undefined,
    "frimousse-row": "",
    style: {
      contain: !sizer ? "content" : undefined,
      height: !sizer ? "var(--frimousse-row-height)" : undefined,
      display: "flex",
    },
  };
}

function listCategoryProps(
  categoryIndex: number,
  category?: EmojiPickerDataCategory,
): WithAttributes<ComponentProps<"div">> {
  return {
    "frimousse-category": "",
    style: {
      contain: "content",
      top: category
        ? `calc(${categoryIndex} * var(--frimousse-category-header-height) + ${category.startRowIndex} * var(--frimousse-row-height))`
        : undefined,
      height: category
        ? `calc(var(--frimousse-category-header-height) + ${category.rowsCount} * var(--frimousse-row-height))`
        : undefined,
      width: "100%",
      pointerEvents: "none",
      position: "absolute",
    },
  };
}

function listCategoryHeaderProps(
  category: EmojiPickerCategory,
  sizer = false,
  sticky = true,
): WithAttributes<EmojiPickerListCategoryHeaderProps> {
  return {
    category,
    "frimousse-category-header": "",
    style: {
      contain: !sizer ? "layout paint" : undefined,
      height: !sizer ? "var(--frimousse-category-header-height)" : undefined,
      pointerEvents: "auto",
      position: sticky ? "sticky" : undefined,
      top: 0,
    },
  };
}

function listSizerProps(
  rowsCount: number,
  categoriesCount: number,
  viewportStartRowIndex: number,
  previousHeadersCount: number,
): WithAttributes<ComponentProps<"div">> {
  return {
    "frimousse-list-sizer": "",
    style: {
      position: "relative",
      boxSizing: "border-box",
      height: `calc(${rowsCount} * var(--frimousse-row-height) + ${categoriesCount} * var(--frimousse-category-header-height))`,
      paddingTop: `calc(${viewportStartRowIndex} * var(--frimousse-row-height) + ${previousHeadersCount} * var(--frimousse-category-header-height))`,
    },
  };
}

function listProps(
  columns: number,
  rowsCount: number,
  style: CSSProperties | undefined,
): WithAttributes<EmojiPickerListProps> {
  return {
    "aria-colcount": columns,
    "aria-rowcount": rowsCount,
    "frimousse-list": "",
    style: {
      "--frimousse-list-columns": columns,
      ...style,
    } as CSSProperties,
    role: "grid",
  };
}

function preventDefault(event: ReactSyntheticEvent) {
  event.preventDefault();
}

const EmojiPickerListEmoji = memo(
  ({
    Emoji,
    emoji,
    columnIndex,
    rowIndex,
  }: {
    emoji: EmojiPickerEmoji;
    columnIndex: number;
    rowIndex: number;
  } & Pick<EmojiPickerListComponents, "Emoji">) => {
    const store = useEmojiPickerStore();
    const isActive = useSelector(
      store,
      (state) => $activeEmoji(state)?.emoji === emoji.emoji,
    );

    const handleSelect = useCallback(() => {
      store.get().onEmojiSelect(emoji);
    }, [emoji]);

    const handlePointerEnter = useCallback(() => {
      store.get().onActiveEmojiChange("pointer", columnIndex, rowIndex);
    }, [columnIndex, rowIndex]);

    const handlePointerLeave = useCallback(() => {
      store.get().onActiveEmojiReset();
    }, []);

    return (
      <Emoji
        {...listEmojiProps(emoji, columnIndex, isActive)}
        onClick={handleSelect}
        onPointerDown={preventDefault}
        onPointerEnter={handlePointerEnter}
        onPointerLeave={handlePointerLeave}
      />
    );
  },
);

const EmojiPickerListRow = memo(
  ({
    Row,
    Emoji,
    rowIndex,
  }: { rowIndex: number } & Pick<
    EmojiPickerListComponents,
    "Emoji" | "Row"
  >) => {
    const store = useEmojiPickerStore();
    const row = useSelector(
      store,
      (state) => state.data?.rows[rowIndex],
      sameEmojiPickerRow,
    );

    /* v8 ignore next 3 */
    if (!row) {
      return null;
    }

    return (
      <Row {...listRowProps(rowIndex)}>
        {row.emojis.map((emoji, columnIndex) => (
          <EmojiPickerListEmoji
            columnIndex={columnIndex}
            Emoji={Emoji}
            emoji={emoji}
            key={emoji.label}
            rowIndex={rowIndex}
          />
        ))}
      </Row>
    );
  },
);

const EmojiPickerListCategory = memo(
  ({
    CategoryHeader,
    categoryIndex,
  }: { categoryIndex: number } & Pick<
    EmojiPickerListComponents,
    "CategoryHeader"
  >) => {
    const store = useEmojiPickerStore();
    const category = useSelector(
      store,
      (state) => state.data?.categories[categoryIndex],
      shallow,
    );
    const sticky = useSelectorKey(store, "sticky");

    /* v8 ignore next 3 */
    if (!category) {
      return null;
    }

    return (
      <div {...listCategoryProps(categoryIndex, category)}>
        <CategoryHeader
          {...listCategoryHeaderProps({ label: category.label }, false, sticky)}
        />
      </div>
    );
  },
);

const EmojiPickerListSizers = memo(
  ({
    CategoryHeader,
    Row,
    Emoji,
  }: Pick<EmojiPickerListComponents, "CategoryHeader" | "Row" | "Emoji">) => {
    const ref = useRef<HTMLDivElement>(null!);
    const store = useEmojiPickerStore();
    const columns = useSelectorKey(store, "columns");
    const emojis = useMemo(
      () =>
        Array<EmojiPickerEmoji>(columns).fill({
          emoji: "🙂",
          label: "",
        }),
      [columns],
    );
    const category: EmojiPickerCategory = useMemo(
      () => ({
        label: "Category",
      }),
      [],
    );
    const rowRef = useRef<HTMLDivElement>(null!);
    const categoryHeaderRef = useRef<HTMLDivElement>(null!);

    useLayoutEffect(() => {
      const list = ref.current?.parentElement?.parentElement;

      /* v8 ignore next 3 */
      if (!list || !rowRef.current || !categoryHeaderRef.current) {
        return;
      }

      const resizeObserver = new ResizeObserver((entries) => {
        for (const entry of entries) {
          const height = entry.contentRect.height;

          const {
            onRowHeightChange,
            onCategoryHeaderHeightChange,
            rowHeight,
            categoryHeaderHeight,
          } = store.get();

          if (entry.target === rowRef.current && rowHeight !== height) {
            onRowHeightChange(height);
          }

          if (
            entry.target === categoryHeaderRef.current &&
            categoryHeaderHeight !== height
          ) {
            onCategoryHeaderHeightChange(height);
          }
        }
      });

      resizeObserver.observe(list);
      resizeObserver.observe(rowRef.current);
      resizeObserver.observe(categoryHeaderRef.current);

      const { onRowHeightChange, onCategoryHeaderHeightChange } = store.get();

      onRowHeightChange(rowRef.current.clientHeight);
      onCategoryHeaderHeightChange(categoryHeaderRef.current.clientHeight);

      return () => {
        resizeObserver.disconnect();
      };
    }, []);

    return (
      <div
        aria-hidden
        ref={ref}
        style={{
          height: 0,
          visibility: "hidden",
        }}
      >
        <div frimousse-row-sizer="" ref={rowRef}>
          <Row {...listRowProps(-1, true)}>
            {emojis.map((emoji, index) => (
              <Emoji key={index} {...listEmojiProps(emoji, index, false)} />
            ))}
          </Row>
        </div>
        <div {...listCategoryProps(-1)}>
          <div frimousse-category-header-sizer="" ref={categoryHeaderRef}>
            <CategoryHeader {...listCategoryHeaderProps(category, true)} />
          </div>
        </div>
      </div>
    );
  },
);

function DefaultEmojiPickerListCategoryHeader({
  category,
  ...props
}: EmojiPickerListCategoryHeaderProps) {
  return <div {...props}>{category.label}</div>;
}

function DefaultEmojiPickerListEmoji({
  emoji,
  ...props
}: EmojiPickerListEmojiProps) {
  return (
    <button type="button" {...props}>
      {emoji.emoji}
    </button>
  );
}

function DefaultEmojiPickerListRow({ ...props }: EmojiPickerListRowProps) {
  return <div {...props} />;
}

/**
 * The list of emojis.
 *
 * @example
 * ```tsx
 * <EmojiPicker.Root>
 *   <EmojiPicker.Search />
 *   <EmojiPicker.Viewport>
 *     <EmojiPicker.List />
 *   </EmojiPicker.Viewport>
 * </EmojiPicker.Root>
 * ```
 *
 * Inner components within the list can be customized via the `components` prop.
 *
 * @example
 * ```tsx
 * <EmojiPicker.List
 *   components={{
 *     CategoryHeader: ({ category, ...props }) => (
 *       <div {...props}>{category.label}</div>
 *     ),
 *     Emoji: ({ emoji, ...props }) => (
 *       <button {...props}>
 *         {emoji.emoji}
 *       </button>
 *     ),
 *     Row: ({ children, ...props }) => <div {...props}>{children}</div>,
 *   }}
 * />
 * ```
 */
const EmojiPickerList = forwardRef<HTMLDivElement, EmojiPickerListProps>(
  ({ style, components, ...props }, forwardedRef) => {
    const store = useEmojiPickerStore();
    const ref = useRef<HTMLDivElement>(null!);
    const callbackRef = useCallback((element: HTMLDivElement | null) => {
      if (element) {
        ref.current = element;
        store.set({ listRef: ref });
      }
    }, []);
    const CategoryHeader =
      components?.CategoryHeader ?? DefaultEmojiPickerListCategoryHeader;
    const Emoji = components?.Emoji ?? DefaultEmojiPickerListEmoji;
    const Row = components?.Row ?? DefaultEmojiPickerListRow;
    const columns = useSelectorKey(store, "columns");
    const viewportStartRowIndex = useSelectorKey(
      store,
      "viewportStartRowIndex",
    );
    const viewportEndRowIndex = useSelectorKey(store, "viewportEndRowIndex");
    const rowsCount = useSelector(store, $rowsCount);
    const categoriesRowsStartIndices = useSelector(
      store,
      $categoriesRowsStartIndices,
      shallow,
    );
    const previousHeadersCount = useMemo(() => {
      return (
        categoriesRowsStartIndices?.filter(
          (index) => index < viewportStartRowIndex,
        ).length ?? 0
      );
    }, [categoriesRowsStartIndices, viewportStartRowIndex]);
    const categoriesCount = categoriesRowsStartIndices?.length ?? 0;

    useImperativeHandle(forwardedRef, () => ref.current);

    if (!rowsCount || !categoriesRowsStartIndices || categoriesCount === 0) {
      return (
        <div {...listProps(columns, 0, style)} {...props}>
          <div {...listSizerProps(0, 0, 0, 0)}>
            <EmojiPickerListSizers
              CategoryHeader={CategoryHeader}
              Emoji={Emoji}
              Row={Row}
            />
          </div>
        </div>
      );
    }

    return (
      <div
        {...listProps(columns, rowsCount, style)}
        {...props}
        ref={callbackRef}
      >
        <div
          {...listSizerProps(
            rowsCount,
            categoriesCount,
            viewportStartRowIndex,
            previousHeadersCount,
          )}
        >
          <EmojiPickerListSizers
            CategoryHeader={CategoryHeader}
            Emoji={Emoji}
            Row={Row}
          />
          {Array.from(
            { length: viewportEndRowIndex - viewportStartRowIndex + 1 },
            (_, index) => {
              const rowIndex = viewportStartRowIndex + index;
              const categoryIndex =
                categoriesRowsStartIndices.indexOf(rowIndex);

              return (
                <Fragment key={rowIndex}>
                  {categoryIndex >= 0 && (
                    <div
                      style={{
                        height: "var(--frimousse-category-header-height)",
                      }}
                    />
                  )}
                  <EmojiPickerListRow
                    Emoji={Emoji}
                    Row={Row}
                    rowIndex={rowIndex}
                  />
                </Fragment>
              );
            },
          )}
          {Array.from({ length: categoriesCount }, (_, index) => (
            <EmojiPickerListCategory
              CategoryHeader={CategoryHeader}
              categoryIndex={index}
              key={index}
            />
          ))}
        </div>
      </div>
    );
  },
);

/**
 * A button to change the current skin tone by cycling through the
 * available skin tones.
 *
 * @example
 * ```tsx
 * <EmojiPicker.SkinToneSelector />
 * ```
 *
 * The emoji used as visual can be customized (by default, ✋).
 *
 * @example
 * ```tsx
 * <EmojiPicker.SkinToneSelector emoji="👋" />
 * ```
 *
 * @see
 * If you want to build a custom skin tone selector, you can use the
 * {@link EmojiPickerSkinTone|`<EmojiPicker.SkinTone />`} component or
 * {@link useSkinTone|`useSkinTone`} hook.
 */
const EmojiPickerSkinToneSelector = forwardRef<
  HTMLButtonElement,
  EmojiPickerSkinToneSelectorProps
>(
  (
    { emoji, onClick, "aria-label": ariaLabel = "Change skin tone", ...props },
    forwardedRef,
  ) => {
    const store = useEmojiPickerStore();
    const skinTones = useSelector(store, $skinTones, shallow);
    const [skinTone, setSkinTone, skinToneVariations] = useSkinTone(emoji);

    const skinToneVariationIndex = useMemo(
      () =>
        Math.max(
          0,
          skinToneVariations.findIndex(
            (variation) => variation.skinTone === skinTone,
          ),
        ),
      [skinTone, skinToneVariations],
    );

    const skinToneVariation = skinToneVariations[skinToneVariationIndex]!;
    const nextSkinToneVariation =
      skinToneVariations[
        (skinToneVariationIndex + 1) % skinToneVariations.length
      ]!;
    const nextSkinTone = nextSkinToneVariation.skinTone;

    const nextSkinToneLabel =
      nextSkinTone === "none" ? undefined : skinTones?.[nextSkinTone];

    const handleClick = useCallback(
      (event: ReactMouseEvent<HTMLButtonElement>) => {
        onClick?.(event);

        if (!event.isDefaultPrevented()) {
          setSkinTone(nextSkinTone);
        }
      },
      [onClick, setSkinTone, nextSkinTone],
    );

    return (
      <button
        type="button"
        {...props}
        aria-label={
          ariaLabel + (nextSkinToneLabel ? ` (${nextSkinToneLabel})` : "")
        }
        aria-live="polite"
        frimousse-skin-tone-selector=""
        onClick={handleClick}
        ref={forwardedRef}
      >
        {skinToneVariation.emoji}
      </button>
    );
  },
);

/**
 * Only renders when the emoji data is loading.
 *
 * @example
 * ```tsx
 * <EmojiPicker.Root>
 *   <EmojiPicker.Search />
 *   <EmojiPicker.Viewport>
 *     <EmojiPicker.Loading>Loading…</EmojiPicker.Loading>
 *     <EmojiPicker.List />
 *   </EmojiPicker.Viewport>
 * </EmojiPicker.Root>
 * ```
 */
function EmojiPickerLoading({ children, ...props }: EmojiPickerLoadingProps) {
  const store = useEmojiPickerStore();
  const isLoading = useSelector(store, $isLoading);

  if (!isLoading) {
    return null;
  }

  return (
    <span frimousse-loading="" {...props}>
      {children}
    </span>
  );
}

function EmojiPickerEmptyWithSearch({
  children,
}: {
  children: (props: { search: string }) => ReactNode;
}) {
  const store = useEmojiPickerStore();
  const search = useSelector(store, $search);

  return children({ search });
}

/**
 * Only renders when no emoji is found for the current search. The content is
 * rendered without any surrounding DOM element.
 *
 * @example
 * ```tsx
 * <EmojiPicker.Root>
 *   <EmojiPicker.Search />
 *   <EmojiPicker.Viewport>
 *     <EmojiPicker.Empty>No emoji found.</EmojiPicker.Empty>
 *     <EmojiPicker.List />
 *   </EmojiPicker.Viewport>
 * </EmojiPicker.Root>
 * ```
 *
 * It can also expose the current search via a render callback to build
 * a more detailed empty state.
 *
 *  @example
 * ```tsx
 * <EmojiPicker.Empty>
 *   {({ search }) => <>No emoji found for "{search}"</>}
 * </EmojiPicker.Empty>
 * ```
 */
function EmojiPickerEmpty({ children, ...props }: EmojiPickerEmptyProps) {
  const store = useEmojiPickerStore();
  const isEmpty = useSelector(store, $isEmpty);

  if (!isEmpty) {
    return null;
  }

  return (
    <span frimousse-empty="" {...props}>
      {typeof children === "function" ? (
        <EmojiPickerEmptyWithSearch>{children}</EmojiPickerEmptyWithSearch>
      ) : (
        children
      )}
    </span>
  );
}

/**
 * Exposes the currently active emoji (either hovered or selected
 * via keyboard navigation) via a render callback.
 *
 * @example
 * ```tsx
 * <EmojiPicker.ActiveEmoji>
 *   {({ emoji }) => <span>{emoji}</span>}
 * </EmojiPicker.ActiveEmoji>
 * ```
 *
 * It can be used to build a preview area next to the list.
 *
 * @example
 * ```tsx
 * <EmojiPicker.ActiveEmoji>
 *   {({ emoji }) => (
 *     <div>
 *       {emoji ? (
 *         <span>{emoji.emoji} {emoji.label}</span>
 *       ) : (
 *         <span>Select an emoji…</span>
 *       )}
 *     </div>
 *   )}
 * </EmojiPicker.ActiveEmoji>
 * ```
 *
 * @see
 * If you prefer to use a hook rather than a component,
 * {@link useActiveEmoji} is also available.
 */
function EmojiPickerActiveEmoji({ children }: EmojiPickerActiveEmojiProps) {
  const activeEmoji = useActiveEmoji();

  return children({ emoji: activeEmoji });
}

/**
 * Exposes the current skin tone and a function to change it via a render
 * callback.
 *
 * @example
 * ```tsx
 * <EmojiPicker.SkinTone>
 *   {({ skinTone, setSkinTone }) => (
 *     <div>
 *       <span>{skinTone}</span>
 *       <button onClick={() => setSkinTone("none")}>Reset skin tone</button>
 *     </div>
 *   )}
 * </EmojiPicker.SkinTone>
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
 * <EmojiPicker.SkinTone emoji="👋">
 *   {({ skinTone, setSkinTone, skinToneVariations }) => (
 *     skinToneVariations.map(({ skinTone, emoji }) => (
 *       <button key={skinTone} onClick={() => setSkinTone(skinTone)}>
 *         {emoji}
 *       </button>
 *     ))
 *   )}
 * </EmojiPicker.SkinTone>
 * ```
 *
 * @see
 * If you prefer to use a hook rather than a component,
 * {@link useSkinTone} is also available.
 *
 * @see
 * An already-built skin tone selector is also available,
 * {@link EmojiPicker.SkinToneSelector|`<EmojiPicker.SkinToneSelector />`}.
 */
function EmojiPickerSkinTone({ children, emoji }: EmojiPickerSkinToneProps) {
  const [skinTone, setSkinTone, skinToneVariations] = useSkinTone(emoji);

  return children({ skinTone, setSkinTone, skinToneVariations });
}

EmojiPickerRoot.displayName = "EmojiPicker.Root";
EmojiPickerSearch.displayName = "EmojiPicker.Search";
EmojiPickerViewport.displayName = "EmojiPicker.Viewport";
EmojiPickerList.displayName = "EmojiPicker.List";
EmojiPickerLoading.displayName = "EmojiPicker.Loading";
EmojiPickerEmpty.displayName = "EmojiPicker.Empty";
EmojiPickerSkinToneSelector.displayName = "EmojiPicker.SkinToneSelector";
EmojiPickerActiveEmoji.displayName = "EmojiPicker.ActiveEmoji";
EmojiPickerSkinTone.displayName = "EmojiPicker.SkinTone";

export {
  EmojiPickerRoot as Root, //                         <EmojiPicker.Root />
  EmojiPickerSearch as Search, //                     <EmojiPicker.Search />
  EmojiPickerViewport as Viewport, //                 <EmojiPicker.Viewport />
  EmojiPickerList as List, //                         <EmojiPicker.List />
  EmojiPickerLoading as Loading, //                   <EmojiPicker.Loading />
  EmojiPickerEmpty as Empty, //                       <EmojiPicker.Empty />
  EmojiPickerSkinToneSelector as SkinToneSelector, // <EmojiPicker.SkinToneSelector />
  EmojiPickerActiveEmoji as ActiveEmoji, //           <EmojiPicker.ActiveEmoji />
  EmojiPickerSkinTone as SkinTone, //                 <EmojiPicker.SkinTone />
};
