import Button from "@material-ui/core/Button"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import useMediaQuery from "@material-ui/core/useMediaQuery"
import KeyboardArrowLeft from "@material-ui/icons/KeyboardArrowLeft"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Maybe } from "components/Conditionals/Maybe"
import { CSSProperties } from "react"
import { PageButton } from "./PageButton"
import { buildPagedList, DEFAULT_RECORDS_PER_PAGE } from "./utils"

export type PaginationWidgetProps = {
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
}: PaginationWidgetProps): JSX.Element | null => {
  const numPages = numRecords ? Math.ceil(numRecords / numRecordsPerPage) : 0
  const firstPageActive = activePage === 1 && numPages !== 0
  const lastPageActive = activePage === numPages && numPages !== 0
  const theme = useTheme()
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"))
  const styles = useStyles()

  // No need to display any pagination if we know the number of pages is 1
  if (numPages === 1 || numRecords === 0) {
    return null
  }

  return (
    <div style={containerStyle} className={styles.defaultContainerStyles}>
      <Button
        className={styles.prevLabelStyles}
        aria-label="Previous page"
        disabled={firstPageActive}
        onClick={onPrevClick}
      >
        <KeyboardArrowLeft />
        <div>{prevLabel}</div>
      </Button>
      <Maybe condition={numPages > 0}>
        <ChooseOne>
          <Cond condition={isMobile}>
            <PageButton
              activePage={activePage}
              page={activePage}
              numPages={numPages}
            />
          </Cond>
          <Cond>
            {buildPagedList(numPages, activePage).map((page) =>
              typeof page !== "number" ? (
                <PageButton
                  key={`Page${page}`}
                  placeholder="..."
                  disabled
                />
              ) : (
                <PageButton
                  key={`Page${page}`}
                  activePage={activePage}
                  page={page}
                  numPages={numPages}
                  onPageClick={onPageClick}
                />
              ),
            )}
          </Cond>
        </ChooseOne>
      </Maybe>
      <Button
        aria-label="Next page"
        disabled={lastPageActive}
        onClick={onNextClick}
      >
        <div>{nextLabel}</div>
        <KeyboardArrowRight />
      </Button>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  defaultContainerStyles: {
    justifyContent: "center",
    alignItems: "center",
    display: "flex",
    flexDirection: "row",
    padding: "20px",
  },

  prevLabelStyles: {
    marginRight: `${theme.spacing(0.5)}px`,
  },

}))
