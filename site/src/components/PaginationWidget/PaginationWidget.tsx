import Button from "@material-ui/core/Button"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import useMediaQuery from "@material-ui/core/useMediaQuery"
import KeyboardArrowLeft from "@material-ui/icons/KeyboardArrowLeft"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import { useActor } from "@xstate/react"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Maybe } from "components/Conditionals/Maybe"
import { CSSProperties } from "react"
import { PaginationMachineRef } from "xServices/pagination/paginationXService"
import { PageButton } from "./PageButton"
import { buildPagedList } from "./utils"

export type PaginationWidgetProps = {
  prevLabel?: string
  nextLabel?: string
  numRecords?: number
  containerStyle?: CSSProperties
  paginationRef: PaginationMachineRef
}

export const PaginationWidget = ({
  prevLabel = "",
  nextLabel = "",
  numRecords,
  containerStyle,
  paginationRef,
}: PaginationWidgetProps): JSX.Element | null => {
  const theme = useTheme()
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"))
  const styles = useStyles()
  const [paginationState, send] = useActor(paginationRef)

  const currentPage = paginationState.context.page
  const numRecordsPerPage = paginationState.context.limit

  const numPages = numRecords ? Math.ceil(numRecords / numRecordsPerPage) : 0
  const firstPageActive = currentPage === 1 && numPages !== 0
  const lastPageActive = currentPage === numPages && numPages !== 0

  // No need to display any pagination if we know the number of pages is 1 or 0
  if (numPages <= 1 || numRecords === 0) {
    return null
  }

  return (
    <div style={containerStyle} className={styles.defaultContainerStyles}>
      <Button
        className={styles.prevLabelStyles}
        aria-label="Previous page"
        disabled={firstPageActive}
        onClick={() => send({ type: "PREVIOUS_PAGE" })}
      >
        <KeyboardArrowLeft />
        <div>{prevLabel}</div>
      </Button>
      <Maybe condition={numPages > 0}>
        <ChooseOne>
          <Cond condition={isMobile}>
            <PageButton
              activePage={currentPage}
              page={currentPage}
              numPages={numPages}
            />
          </Cond>
          <Cond>
            {buildPagedList(numPages, currentPage).map((page) =>
              typeof page !== "number" ? (
                <PageButton
                  key={`Page${page}`}
                  activePage={currentPage}
                  placeholder="..."
                  disabled
                />
              ) : (
                <PageButton
                  key={`Page${page}`}
                  activePage={currentPage}
                  page={page}
                  numPages={numPages}
                  onPageClick={() => send({ type: "GO_TO_PAGE", page })}
                />
              ),
            )}
          </Cond>
        </ChooseOne>
      </Maybe>
      <Button
        aria-label="Next page"
        disabled={lastPageActive}
        onClick={() => send({ type: "NEXT_PAGE" })}
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
