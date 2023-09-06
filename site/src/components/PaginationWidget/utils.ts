import {
  PaginationContext,
  paginationMachine,
  PaginationMachineRef,
} from "xServices/pagination/paginationXService";
import { spawn } from "xstate";

/**
 * Generates a ranged array with an option to step over values.
 * Shamelessly stolen from:
 * https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/from#sequence_generator_range
 */
const range = (start: number, stop: number, step = 1) =>
  Array.from({ length: (stop - start) / step + 1 }, (_, i) => start + i * step);

export const DEFAULT_RECORDS_PER_PAGE = 25;
// Number of pages to the left or right of the current page selection.
const PAGE_NEIGHBORS = 1;
// Number of pages displayed for cases where there are multiple ellipsis showing. This can be
// thought of as the minimum number of page numbers to display when multiple ellipsis are showing.
const PAGES_TO_DISPLAY = PAGE_NEIGHBORS * 2 + 3;
// Total page blocks(page numbers or ellipsis) displayed, including the maximum number of ellipsis (2).
// This gives us maximum number of 7 page blocks to be displayed when the page neighbors value is 1.
const NUM_PAGE_BLOCKS = PAGES_TO_DISPLAY + 2;

/**
 * Builds a list of pages based on how many pages exist and where the user is in their navigation of those pages.
 * List result is used to from the buttons that make up the Pagination Widget
 */
export const buildPagedList = (
  numPages: number,
  activePage: number,
): ("left" | "right" | number)[] => {
  if (numPages > NUM_PAGE_BLOCKS) {
    let pages = [];
    const leftBound = activePage - PAGE_NEIGHBORS;
    const rightBound = activePage + PAGE_NEIGHBORS;
    const beforeLastPage = numPages - 1;
    const startPage = leftBound > 2 ? leftBound : 2;
    const endPage = rightBound < beforeLastPage ? rightBound : beforeLastPage;

    pages = range(startPage, endPage);

    const singleSpillOffset = PAGES_TO_DISPLAY - pages.length - 1;
    const hasLeftOverflow = startPage > 2;
    const hasRightOverflow = endPage < beforeLastPage;
    const leftOverflowPage = "left" as const;
    const rightOverflowPage = "right" as const;

    if (hasLeftOverflow && !hasRightOverflow) {
      const extraPages = range(startPage - singleSpillOffset, startPage - 1);
      pages = [leftOverflowPage, ...extraPages, ...pages];
    } else if (!hasLeftOverflow && hasRightOverflow) {
      const extraPages = range(endPage + 1, endPage + singleSpillOffset);
      pages = [...pages, ...extraPages, rightOverflowPage];
    } else if (hasLeftOverflow && hasRightOverflow) {
      pages = [leftOverflowPage, ...pages, rightOverflowPage];
    }

    return [1, ...pages, numPages];
  }

  return range(1, numPages);
};

const getInitialPage = (page: string | null): number =>
  page ? Number(page) : 1;

// pages count from 1
export const getOffset = (page: number, limit: number): number =>
  (page - 1) * limit;

interface PaginationData {
  offset: number;
  limit: number;
}

export const getPaginationData = (
  ref: PaginationMachineRef,
): PaginationData => {
  const snapshot = ref.getSnapshot();
  if (snapshot) {
    const { page, limit } = snapshot.context;
    const offset = getOffset(page, limit);
    return { offset, limit };
  } else {
    throw new Error("No pagination data");
  }
};

export const getPaginationContext = (
  searchParams: URLSearchParams,
  limit: number = DEFAULT_RECORDS_PER_PAGE,
): PaginationContext => ({
  page: getInitialPage(searchParams.get("page")),
  limit,
});

// for storybook
export const createPaginationRef = (
  context: PaginationContext,
): PaginationMachineRef => {
  return spawn(paginationMachine.withContext(context));
};

export const nonInitialPage = (searchParams: URLSearchParams): boolean => {
  const page = searchParams.get("page");
  const numberPage = page ? Number(page) : 1;
  return numberPage > 1;
};
