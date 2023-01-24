import Button from "@material-ui/core/Button"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import useMediaQuery from "@material-ui/core/useMediaQuery"
import KeyboardArrowLeft from "@material-ui/icons/KeyboardArrowLeft"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { PageButton } from "./PageButton"
import { buildPagedList } from "./utils"

export type PaginationWidgetBaseProps = {
  count: number
  page: number
  limit: number
  onChange: (page: number) => void
}

export const PaginationWidgetBase = ({
  count,
  page,
  limit,
  onChange,
}: PaginationWidgetBaseProps): JSX.Element | null => {
  const theme = useTheme()
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"))
  const styles = useStyles()
  const numPages = Math.ceil(count / limit)
  const isFirstPage = page === 0
  const isLastPage = page === numPages - 1

  if (numPages < 2) {
    return null
  }

  return (
    <div className={styles.defaultContainerStyles}>
      <Button
        className={styles.prevLabelStyles}
        aria-label="Previous page"
        disabled={isFirstPage}
        onClick={() => {
          if (!isFirstPage) {
            onChange(page - 1)
          }
        }}
      >
        <KeyboardArrowLeft />
      </Button>
      <ChooseOne>
        <Cond condition={isMobile}>
          <PageButton activePage={page} page={page} numPages={numPages} />
        </Cond>
        <Cond>
          {buildPagedList(numPages, page).map((pageItem) => {
            if (pageItem === "left" || pageItem === "right") {
              return (
                <PageButton
                  key={pageItem}
                  activePage={page}
                  placeholder="..."
                  disabled
                />
              )
            }

            return (
              <PageButton
                key={pageItem}
                page={pageItem}
                activePage={page}
                numPages={numPages}
                onPageClick={() => onChange(pageItem)}
              />
            )
          })}
        </Cond>
      </ChooseOne>
      <Button
        aria-label="Next page"
        disabled={isLastPage}
        onClick={() => {
          if (!isLastPage) {
            onChange(page + 1)
          }
        }}
      >
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
