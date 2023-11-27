import {
  type FC,
  type HTMLAttributes,
  type MouseEvent as ReactMouseEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";

import { useTheme } from "@emotion/react";
import { useEffectEvent } from "hooks/hookPolyfills";
import { type UsePaginatedQueryResult } from "hooks/usePaginatedQuery";

import { PaginationWidgetBase } from "./PaginationWidgetBase";
import Skeleton from "@mui/material/Skeleton";

type PaginationProps = HTMLAttributes<HTMLDivElement> & {
  paginationResult: UsePaginatedQueryResult;
  paginationUnitLabel: string;

  /**
   * Mainly here to simplify Storybook integrations. This should almost always
   * be true in production
   */
  autoScroll?: boolean;
};

const userInteractionEvents: (keyof WindowEventMap)[] = [
  "click",
  "scroll",
  "pointerenter",
  "touchstart",
  "keydown",
];

export const Pagination: FC<PaginationProps> = ({
  children,
  paginationResult,
  paginationUnitLabel,
  autoScroll = true,
  ...delegatedProps
}) => {
  const scrollContainerProps = useScrollOnPageChange(
    paginationResult.currentPage,
    paginationResult.isPreviousData,
    autoScroll,
  );

  return (
    <div {...scrollContainerProps}>
      <PaginationHeader
        paginationResult={paginationResult}
        paginationUnitLabel={paginationUnitLabel}
      />

      <div
        css={{
          display: "flex",
          flexFlow: "column nowrap",
          rowGap: "16px",
        }}
        {...delegatedProps}
      >
        {children}

        {paginationResult.isSuccess && (
          <PaginationWidgetBase
            totalRecords={paginationResult.totalRecords}
            currentPage={paginationResult.currentPage}
            pageSize={paginationResult.limit}
            onPageChange={paginationResult.onPageChange}
            hasPreviousPage={paginationResult.hasPreviousPage}
            hasNextPage={paginationResult.hasNextPage}
          />
        )}
      </div>
    </div>
  );
};

type PaginationHeaderProps = {
  paginationResult: UsePaginatedQueryResult;
  paginationUnitLabel: string;
};

const PaginationHeader: FC<PaginationHeaderProps> = ({
  paginationResult,
  paginationUnitLabel,
}) => {
  const theme = useTheme();
  const endBound = Math.min(
    paginationResult.limit - 1,
    (paginationResult.totalRecords ?? 0) - (paginationResult.currentChunk ?? 0),
  );

  return (
    <div
      css={{
        display: "flex",
        flexFlow: "row nowrap",
        alignItems: "center",
        margin: 0,
        fontSize: "13px",
        paddingBottom: "8px",
        color: theme.palette.text.secondary,
        height: "36px", // The size of a small button
        "& strong": {
          color: theme.palette.text.primary,
        },
      }}
    >
      {!paginationResult.isSuccess ? (
        <Skeleton variant="text" width={160} height={16} />
      ) : (
        <div>
          Showing {paginationUnitLabel}{" "}
          <strong>
            {paginationResult.currentChunk}&ndash;
            {paginationResult.currentChunk + endBound}
          </strong>{" "}
          (<strong>{paginationResult.totalRecords}</strong>{" "}
          {paginationUnitLabel} total)
        </div>
      )}
    </div>
  );
};

/**
 * Splitting this into a custom hook because there's a lot of convoluted logic
 * here (the use case doesn't line up super well with useEffect, even though
 * it's the only tool that solves the problem). Please do not export this; it
 * should be treated as an internal implementation detail
 *
 * Scrolls the user to the top of the pagination container when the current
 * page changes (accounting for old data being shown during loading transitions)
 *
 * See Pagination test file for all cases this is meant to handle
 */
function useScrollOnPageChange(
  currentPage: number,
  showingPreviousData: boolean,
  autoScroll: boolean,
) {
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const isScrollingQueuedRef = useRef(false);

  // Sets up event handlers for canceling queued scrolls in response to
  // literally any user interaction
  useEffect(() => {
    if (!autoScroll) {
      return;
    }

    const cancelScroll = () => {
      isScrollingQueuedRef.current = false;
    };

    for (const event of userInteractionEvents) {
      window.addEventListener(event, cancelScroll);
    }

    return () => {
      for (const event of userInteractionEvents) {
        window.removeEventListener(event, cancelScroll);
      }
    };
  }, [autoScroll]);

  const scrollToTop = useEffectEvent(() => {
    const newVerticalPosition =
      (scrollContainerRef.current?.getBoundingClientRect().top ?? 0) +
      window.scrollY;

    // Not using element.scrollIntoView for testing reasons; much easier to mock
    // the global window object
    window.scrollTo({ top: newVerticalPosition, behavior: "instant" });
    isScrollingQueuedRef.current = false;
  });

  // Tracking whether we're on the first render, because calling the effects
  // unconditionally will just hijack the user and feel absolutely awful
  const isOnFirstRenderRef = useRef(true);
  const syncPageChange = useEffectEvent(() => {
    if (isOnFirstRenderRef.current) {
      isOnFirstRenderRef.current = false;
      return;
    }

    if (showingPreviousData) {
      isScrollingQueuedRef.current = true;
    } else {
      scrollToTop();
    }
  });

  // Would've liked to consolidate these effects into a single useLayoutEffect
  // call, but they kept messing each other up when grouped together
  useLayoutEffect(() => {
    syncPageChange();
  }, [syncPageChange, currentPage]);

  useLayoutEffect(() => {
    if (!showingPreviousData && isScrollingQueuedRef.current) {
      scrollToTop();
    }
  }, [scrollToTop, showingPreviousData]);

  /**
   * This is meant to capture and stop event bubbling for events that come from
   * deeper within Pagination
   *
   * Without this, this is the order of operations that happens when you change
   * a page while no data is available for the page you're going to:
   * 1. User uses keyboard/mouse to change page
   * 2. Event handler dispatches state changes to React
   * 3. Even though flushing a state change is async, React will still flush
   *    and re-render before the event is allowed to travel further up
   * 4. The current page triggers the layout effect, queuing a scroll
   * 5. The event resumes bubbling up and reaches the window object
   * 6. The window object unconditionally cancels the scroll, immediately and
   *    always undoing any kind of scroll queuing you try to do
   *
   * One alternative was reaching deeper within the event handlers for the inner
   * components and passing the events directly through multiple layers. Tried
   * it, but it got clunky fast. Better to have the ugliness in one spot
   */
  const stopInternalEventBubbling = (
    event: ReactMouseEvent<unknown, MouseEvent> | ReactKeyboardEvent<unknown>,
  ) => {
    const { nativeEvent } = event;

    const isEventFromClick =
      nativeEvent instanceof MouseEvent ||
      (nativeEvent instanceof KeyboardEvent &&
        (nativeEvent.key === " " || nativeEvent.key === "Enter"));

    const shouldStopBubbling =
      isEventFromClick &&
      !isScrollingQueuedRef.current &&
      event.target instanceof HTMLElement &&
      scrollContainerRef.current !== event.target &&
      scrollContainerRef.current?.contains(event.target);

    if (shouldStopBubbling) {
      event.stopPropagation();
    }
  };

  return {
    ref: scrollContainerRef,
    onClick: stopInternalEventBubbling,
    onKeyDown: stopInternalEventBubbling,
  } as const;
}
