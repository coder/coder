import { type PropsWithChildren, useEffect, useRef } from "react";
import { PaginationWidgetBase } from "./PaginationWidgetBase";

type PaginationProps = PropsWithChildren<{
  fetching: boolean;
  currentPage: number;
  pageSize: number;
  totalRecords: number;
  onPageChange: (newPage: number) => void;
}>;

export function Pagination({
  children,
  fetching,
  currentPage,
  totalRecords,
  pageSize,
  onPageChange,
}: PaginationProps) {
  const scrollAfterPageChangeRef = useRef(false);
  useEffect(() => {
    const onScroll = () => {
      scrollAfterPageChangeRef.current = false;
    };

    document.addEventListener("scroll", onScroll);
    return () => document.removeEventListener("scroll", onScroll);
  }, []);

  const previousPageRef = useRef<number | undefined>(undefined);
  const paginationTopRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const paginationTop = paginationTopRef.current;
    const isInitialRender = previousPageRef.current === undefined;

    const skipScroll =
      isInitialRender ||
      paginationTop === null ||
      !scrollAfterPageChangeRef.current;

    previousPageRef.current = currentPage;
    if (!skipScroll) {
      paginationTop.scrollIntoView();
    }
  }, [currentPage]);

  return (
    <>
      <div ref={paginationTopRef} />
      {children}

      <PaginationWidgetBase
        currentPage={currentPage}
        totalRecords={totalRecords}
        pageSize={pageSize}
        onPageChange={(newPage) => {
          scrollAfterPageChangeRef.current = true;
          onPageChange(newPage);
        }}
      />
    </>
  );
}
