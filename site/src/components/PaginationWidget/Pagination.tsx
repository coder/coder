import {
  type FC,
  type PropsWithChildren,
  useEffect,
  useLayoutEffect,
  useRef,
  useMemo,
} from "react";

import { PaginationWidgetBase } from "./PaginationWidgetBase";
import { throttle } from "lodash";
import { useEffectEvent } from "hooks/hookPolyfills";

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

  /**
   * @todo Probably better just to make a useThrottledFunction custom hook,
   * rather than the weird useEffectEvent+useMemo approach. Cannot use throttle
   * inside the render path directly; it will create a new stateful function
   * every single render, and there won't be a single throttle state
   */
  const cancelScroll = useEffectEvent(() => {
    if (showingPreviousData) {
      scrollCanceledRef.current = true;
    }
  });

  const throttledCancelScroll = useMemo(() => {
    return throttle(cancelScroll, 200);
  }, [cancelScroll]);

  useEffect(() => {
    for (const event of userInteractionEvents) {
      window.addEventListener(event, throttledCancelScroll);
    }

    return () => {
      for (const event of userInteractionEvents) {
        window.removeEventListener(event, throttledCancelScroll);
      }
    };
  }, [throttledCancelScroll]);

  useLayoutEffect(() => {
    scrollCanceledRef.current = false;
  }, [currentPage]);

  useLayoutEffect(() => {
    if (showingPreviousData) {
      return;
    }

    const shouldScroll = autoScroll && !scrollCanceledRef.current;
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
