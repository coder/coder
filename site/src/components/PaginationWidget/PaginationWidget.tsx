import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { Theme } from "@material-ui/core/styles/createMuiTheme"
import KeyboardArrowLeft from "@material-ui/icons/KeyboardArrowLeft"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import { CSSProperties } from "react"

type PaginatedWidgetProps = {
  prevLabel: string
  nextLabel: string
  onPrevClick: () => void
  onNextClick: () => void
  onPageClick?: (page: number) => void
  numRecordsPerPage?: number
  numRecords?: number
  activePage?: number
  containerStyle?: CSSProperties
}

/**
 * Generates a ranged array with an option to step over values.
 * Shamelessly stolen from:
 * https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/from#sequence_generator_range
 */
const range = (start: number, stop: number, step = 1) =>
  Array.from({ length: (stop - start) / step + 1 }, (_, i) => start + i * step)

const DEFAULT_RECORDS_PER_PAGE = 25
// Number of pages to the left or right of the current page selection.
const PAGE_NEIGHBORS = 1
// Number of pages displayed for cases where there are multiple ellipsis showing. This can be
// thought of as the minimum number of page numbers to display when multiple ellipsis are showing.
const PAGES_TO_DISPLAY = PAGE_NEIGHBORS * 2 + 3
// Total page blocks(page numbers or ellipsis) displayed, including the maximum number of ellipsis (2).
// This gives us maximum number of 7 page blocks to be displayed when the page neighbors value is 1.
const NUM_PAGE_BLOCKS = PAGES_TO_DISPLAY + 2

export const PaginationWidget = ({
  prevLabel,
  nextLabel,
  onPrevClick,
  onNextClick,
  onPageClick,
  numRecords,
  numRecordsPerPage = DEFAULT_RECORDS_PER_PAGE,
  activePage = 1,
  containerStyle,
}: PaginatedWidgetProps): JSX.Element | null => {
  const numPages = numRecords ? Math.ceil(numRecords / numRecordsPerPage) : 0
  const firstPageActive = activePage === 1 && numPages !== 0
  const lastPageActive = activePage === numPages && numPages !== 0

  const styles = useStyles({ isActive: true })

  console.log("activePage", activePage)

  console.log("firstPageActive", firstPageActive, lastPageActive)

  /**
   * A paged list will be constructed and can appear as some permutation of the following:
   * [1, 2, 3, 4, 5]
   * [1, 2, 3, 4, 5, 'right']
   * ['left', 6, 7, 8, 9]
   * [1, 2, 'left', 6, 7, 8, 'right', 18]
   */
  const buildPagedList = () => {
    if (numPages > NUM_PAGE_BLOCKS) {
      let pages = []
      const leftBound = activePage - PAGE_NEIGHBORS
      const rightBound = activePage + PAGE_NEIGHBORS
      const beforeLastPage = numPages - 1
      const startPage = leftBound > 2 ? leftBound : 2
      const endPage = rightBound < beforeLastPage ? rightBound : beforeLastPage

      pages = range(startPage, endPage)

      const singleSpillOffset = PAGES_TO_DISPLAY - pages.length - 1
      const hasLeftOverflow = startPage > 2
      const hasRightOverflow = endPage < beforeLastPage
      const leftOverflowPage = "left"
      const rightOverflowPage = "right"

      if (hasLeftOverflow && !hasRightOverflow) {
        const extraPages = range(startPage - singleSpillOffset, startPage - 1)
        pages = [leftOverflowPage, ...extraPages, ...pages]
      } else if (!hasLeftOverflow && hasRightOverflow) {
        const extraPages = range(endPage + 1, endPage + singleSpillOffset)
        pages = [...pages, ...extraPages, rightOverflowPage]
      } else if (hasLeftOverflow && hasRightOverflow) {
        pages = [leftOverflowPage, ...pages, rightOverflowPage]
      }

      return [1, ...pages, numPages]
    }

    return range(1, numPages)
  }

  // No need to display any pagination if we know the number of pages is 1
  if (numPages === 1) {
    return null
  }

  return (
    <div style={containerStyle} className={styles.defaultContainerStyles}>
      <Button aria-label="Previous page" disabled={firstPageActive} onClick={onPrevClick}>
        <KeyboardArrowLeft />
        <div>{prevLabel}</div>
      </Button>
      {numPages > 0 &&
        buildPagedList().map((page) =>
          typeof page !== "number" ? (
            <Button
              style={{ marginLeft: "4px" }} // easiest to use inline-style here because of first-of-type constraints
              className={styles.pageButton}
              key={`Page${page}`}
              disabled
            >
              <div>...</div>
            </Button>
          ) : (
            <Button
              style={{ marginLeft: "4px" }} // easiest to use inline-style here because of first-of-type constraints
              className={
                activePage === page
                  ? `${styles.pageButton} ${styles.activePageButton}`
                  : styles.pageButton
              }
              aria-label={`${page === activePage ? "Current Page" : ""} ${
                page === numPages ? "Last Page" : ""
              } Page${page}`}
              name="Page button"
              key={`Page${page}`}
              onClick={() => onPageClick && onPageClick(page)}
            >
              <div>{page}</div>
            </Button>
          ),
        )}
      <Button aria-label="Next page" disabled={lastPageActive} onClick={onNextClick}>
        <div>{nextLabel}</div>
        <KeyboardArrowRight />
      </Button>
    </div>
  )
}

type StyleProps = {
  isActive?: boolean
}

const useStyles = makeStyles<Theme, StyleProps>((theme) => ({
  defaultContainerStyles: {
    justifyContent: "center",
    alignItems: "center",
    display: "flex",
    flexDirection: "row",
    padding: "20px",
  },

  //   previousLabelText: {
  //     color: ({ firstPageActive }) => {
  //       return firstPageActive ? theme.palette.secondary.contrastText : theme.palette.text.primary
  //     },
  //   },
  //
  //   nextLabelText: {
  //     color: ({ lastPageActive }) => {
  //       return lastPageActive ? theme.palette.secondary.contrastText : theme.palette.text.primary
  //     },
  //   },

  pageButton: {
    height: `${theme.spacing(3)}`,
    width: `${theme.spacing(3)}`,

    "&:not(:last-of-type)": {
      marginRight: theme.spacing(0.5),
    },
  },

  activePageButton: {
    borderColor: `${theme.palette.success.main}`,
    backgroundColor: `${theme.palette.success.dark}`,
  },
}))
