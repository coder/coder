import {
  type FC,
  type HTMLAttributes,
  useEffect,
  useLayoutEffect,
  useRef,
  MouseEvent as ReactMouseEvent,
  KeyboardEvent as ReactKeyboardEvent,
} from "react";

import { useEffectEvent } from "hooks/hookPolyfills";
import { PaginationWidgetBase } from "./PaginationWidgetBase";

type PaginationProps = HTMLAttributes<HTMLDivElement> & {
  currentPage: number;
  pageSize: number;
  totalRecords: number | undefined;
  onPageChange: (newPage: number) => void;

  /**
   * Meant to interface with useQuery's isPreviousData property.
   *
   * Indicates whether data for a previous query is being shown while a new
   * query is loading in
   */
  showingPreviousData: boolean;

  // Mainly here for Storybook integration; autoScroll should almost always be
  // flipped to true in production
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
  currentPage,
  pageSize,
  totalRecords,
  showingPreviousData,
  onPageChange,
  autoScroll = true,
  ...delegatedProps
}) => {
  const scrollContainerProps = useScrollOnPageChange(
    currentPage,
    showingPreviousData,
    autoScroll,
  );

  return (
    <div {...scrollContainerProps}>
      <div
        css={{
          display: "flex",
          flexFlow: "column nowrap",
          rowGap: "24px",
        }}
        {...delegatedProps}
      >
        {children}
        {totalRecords !== undefined && (
          <PaginationWidgetBase
            currentPage={currentPage}
            pageSize={pageSize}
            totalRecords={totalRecords}
            onPageChange={onPageChange}
          />
        )}
      </div>
    </div>
  );
};

/**
 * Splitting this into a custom hook because there's a lot of convoluted logic
 * here (the use case doesn't line up super well with useEffect, even though
 * it's the only tool that solves the problem). Do not export this; it should be
 * treated as an internal implementation detail
 *
 * Scrolls the user to the top of the pagination container when the current
 * page changes (accounting for old data being shown during loading transitions)
 *
 * See component test file for all cases this is meant to handle
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
    scrollContainerRef.current?.scrollIntoView({
      block: "start",
      behavior: "instant",
    });

    isScrollingQueuedRef.current = false;
  });

  // Tracking whether we're on the first render, because calling the effects
  // unconditionally will just hijack the user and make pages feel janky
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
