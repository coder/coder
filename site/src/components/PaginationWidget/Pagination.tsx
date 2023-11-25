import {
  type FC,
  type HTMLAttributes,
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";

import { useEffectEvent } from "hooks/hookPolyfills";
import { PaginationWidgetBase } from "./PaginationWidgetBase";

type PaginationProps = HTMLAttributes<HTMLDivElement> & {
  currentPage: number;
  pageSize: number;
  totalRecords: number | undefined;
  onPageChange: (newPage: number) => void;
  autoScroll?: boolean;
  scrollBehavior?: ScrollBehavior;

  /**
   * Meant to interface with useQuery's isPreviousData property.
   *
   * Indicates whether data for a previous query is being shown while a new
   * query is loading in
   */
  showingPreviousData: boolean;
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
  scrollBehavior = "instant",
  ...delegatedProps
}) => {
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const isDeferredScrollActiveRef = useRef(false);

  useEffect(() => {
    const cancelScroll = () => {
      isDeferredScrollActiveRef.current = false;
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

  const scroll = useEffectEvent(() => {
    if (autoScroll) {
      scrollContainerRef.current?.scrollIntoView({
        block: "start",
        behavior: scrollBehavior,
      });
    }
  });

  const handlePageChange = useEffectEvent(() => {
    if (showingPreviousData) {
      isDeferredScrollActiveRef.current = true;
    } else {
      scroll();
    }
  });

  useLayoutEffect(() => {
    handlePageChange();
  }, [handlePageChange, currentPage]);

  useLayoutEffect(() => {
    if (!showingPreviousData && isDeferredScrollActiveRef.current) {
      scroll();
    }
  }, [scroll, showingPreviousData]);

  return (
    <div ref={scrollContainerRef}>
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
