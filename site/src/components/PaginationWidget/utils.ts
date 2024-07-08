/**
 * Generates a ranged array with an option to step over values.
 * Shamelessly stolen from:
 * {@link https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/from#sequence_generator_range}
 */
const range = (start: number, stop: number, step = 1) =>
  Array.from({ length: (stop - start) / step + 1 }, (_, i) => start + i * step);

export const DEFAULT_RECORDS_PER_PAGE = 25;

// Number of pages to display on either side of the current page selection
const PAGE_NEIGHBORS = 1;

// Minimum number of pages to display at all times (assuming there are enough
// pages). Needed to handle case where the left and right placeholder/ellipsis
// elements are both showing
const PAGES_TO_DISPLAY = PAGE_NEIGHBORS * 2 + 3;

// Total number of pagination elements to display (accounting for visible pages
// and up to two ellipses placeholders). With 1 page neighbor on either side,
// the UI will show up to seven elements total
const TOTAL_PAGE_BLOCKS = PAGES_TO_DISPLAY + 2;

/**
 * Takes the total number of pages from a pagination result, and truncates it
 * into a UI-friendly list.
 */
export const buildPagedList = (
  numPages: number,
  activePage: number,
): ("left" | "right" | number)[] => {
  if (numPages <= TOTAL_PAGE_BLOCKS) {
    return range(1, numPages);
  }

  const isInvalidActivePage = activePage > numPages || activePage < 1;
  const pageBeforeLast = numPages - 1;
  const startPage = isInvalidActivePage
    ? 1 + PAGE_NEIGHBORS
    : Math.max(activePage - PAGE_NEIGHBORS, 2);
  const endPage = isInvalidActivePage
    ? numPages - PAGE_NEIGHBORS
    : Math.min(activePage + PAGE_NEIGHBORS, pageBeforeLast);

  let pages: ReturnType<typeof buildPagedList> = range(startPage, endPage);

  const singleSpillOffset = PAGES_TO_DISPLAY - pages.length - 1;
  const hasLeftOverflow = startPage > 2;
  const hasRightOverflow = endPage < pageBeforeLast;

  if (hasLeftOverflow && !hasRightOverflow) {
    const extraPages = range(startPage - singleSpillOffset, startPage - 1);
    pages = ["left", ...extraPages, ...pages];
  } else if (!hasLeftOverflow && hasRightOverflow) {
    const extraPages = range(endPage + 1, endPage + singleSpillOffset);
    pages = [...pages, ...extraPages, "right"];
  } else if (hasLeftOverflow && hasRightOverflow) {
    pages = ["left", ...pages, "right"];
  }

  return [1, ...pages, numPages];
};

/**
 * Calculates the current offset from the start of a paginated dataset
 */
export const getOffset = (page: number, pageSize: number): number => {
  const pageIsValid = Number.isInteger(page) && page >= 1;
  const pageToUse = pageIsValid ? page : 1;

  return (pageToUse - 1) * pageSize;
};

export const isNonInitialPage = (searchParams: URLSearchParams): boolean => {
  const page = searchParams.get("page");
  const conversion = Number(page);

  return Number.isInteger(conversion) && conversion > 1;
};
