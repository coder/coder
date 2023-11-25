import {
  type FC,
  type PropsWithChildren,
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";

import { useEffectEvent } from "hooks/hookPolyfills";
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
  const scrollCanceledRef = useRef(false);

  const cancelScroll = useEffectEvent(() => {
    if (showingPreviousData) {
      scrollCanceledRef.current = true;
    }
  });

  useEffect(() => {
    for (const event of userInteractionEvents) {
      window.addEventListener(event, cancelScroll);
    }

    return () => {
      for (const event of userInteractionEvents) {
        window.removeEventListener(event, cancelScroll);
      }
    };
  }, [cancelScroll]);

  const handlePageChange = useEffectEvent(() => {
    if (showingPreviousData) {
      scrollCanceledRef.current = false;
      return;
    }

    if (!autoScroll) {
      return;
    }

    scrollContainerRef.current?.scrollIntoView({
      block: "start",
      behavior: "instant",
    });
  });

  useLayoutEffect(() => {
    handlePageChange();
  }, [handlePageChange, currentPage]);

  useLayoutEffect(() => {
    const shouldScroll =
      autoScroll && !showingPreviousData && !scrollCanceledRef.current;

    if (shouldScroll) {
      scrollContainerRef.current?.scrollIntoView({
        block: "start",
        behavior: "instant",
      });
    }
  }, [autoScroll, showingPreviousData]);

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
