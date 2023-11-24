import {
  type FC,
  type PropsWithChildren,
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";

import { PaginationWidgetBase } from "./PaginationWidgetBase";

type PaginationProps = PropsWithChildren<{
  currentPage: number;
  pageSize: number;
  totalRecords: number | undefined;
  onPageChange: (newPage: number) => void;
  autoScroll?: boolean;

  /**
   * Meant to interface with useQuery's isPreviousData property
   *
   * Indicates whether data for a previous query is being shown while a new
   * query is loading in
   */
  showingPreviousData?: boolean;
}>;

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
  onPageChange,
  autoScroll = true,
  showingPreviousData = false,
}) => {
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const scrollAfterDataLoadsRef = useRef(false);

  // Manages event handlers for canceling scrolling if the user interacts with
  // the page in any way while new data is loading in. Don't want to scroll and
  // hijack their browser if they're in the middle of something else!
  useEffect(() => {
    const cancelScroll = () => {
      scrollAfterDataLoadsRef.current = false;
    };

    for (const event of userInteractionEvents) {
      window.addEventListener(event, cancelScroll);
    }

    return () => {
      for (const event of userInteractionEvents) {
        window.removeEventListener(event, cancelScroll);
      }
    };
  }, []);

  // Syncs scroll tracking to page changes. Wanted to handle these changes via a
  // click event handler, but that got overly complicated between making sure
  // that events didn't bubble all the way to the window (where they would
  // immediately be canceled by window), and needing to update all downstream
  // click handlers to be aware of event objects. Must be layout effect in order
  // to fire before layout effect defined below
  const mountedRef = useRef(false);
  useLayoutEffect(() => {
    // Never want to turn scrolling on for initial mount. Tried avoiding ref and
    // checking things like viewport, but they all seemed unreliable (especially
    // if the user can interact with the page while JS is still loading in)
    if (mountedRef.current) {
      mountedRef.current = true;
      return;
    }

    scrollAfterDataLoadsRef.current = true;
  }, [currentPage]);

  // Jumps the user to the top of the paginated container each time new data
  // loads in. Has no dependency array, because you can't sync based off of
  // showingPreviousData. If its value is always false (via default params),
  // an effect synced with it will never fire beyond the on-mount call
  useLayoutEffect(() => {
    const shouldScroll =
      autoScroll && !showingPreviousData && scrollAfterDataLoadsRef.current;

    if (shouldScroll) {
      scrollContainerRef.current?.scrollIntoView({
        block: "start",
        behavior: "instant",
      });
    }
  });

  return (
    <div ref={scrollContainerRef}>
      <div
        css={{
          display: "flex",
          flexFlow: "column nowrap",
          rowGap: "24px",
        }}
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
