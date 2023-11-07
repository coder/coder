import Button from "@mui/material/Button";
import useMediaQuery from "@mui/material/useMediaQuery";
import KeyboardArrowLeft from "@mui/icons-material/KeyboardArrowLeft";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import { useTheme } from "@emotion/react";
import { PageButton } from "./PageButton";
import { buildPagedList } from "./utils";

export type PaginationWidgetBaseProps = {
  count: number;
  page: number;
  limit: number;
  onChange: (page: number) => void;
};

export const PaginationWidgetBase = ({
  count,
  limit,
  onChange,
  page: currentPage,
}: PaginationWidgetBaseProps): JSX.Element | null => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));

  const numPages = Math.ceil(count / limit);
  if (numPages < 2) {
    return null;
  }

  const isFirstPage = currentPage <= 1;
  const isLastPage = currentPage >= numPages;

  return (
    <div
      css={{
        justifyContent: "center",
        alignItems: "center",
        display: "flex",
        flexDirection: "row",
        padding: "20px",
      }}
    >
      <Button
        css={{
          marginRight: 4,
        }}
        aria-label="Previous page"
        disabled={isFirstPage}
        onClick={() => {
          if (!isFirstPage) {
            onChange(currentPage - 1);
          }
        }}
      >
        <KeyboardArrowLeft />
      </Button>
      {isMobile ? (
        <PageButton
          activePage={currentPage}
          page={currentPage}
          numPages={numPages}
        />
      ) : (
        buildPagedList(numPages, currentPage).map((pageItem) => {
          if (pageItem === "left" || pageItem === "right") {
            return (
              <PageButton
                key={pageItem}
                activePage={currentPage}
                placeholder="..."
                disabled
              />
            );
          }

          return (
            <PageButton
              key={pageItem}
              page={pageItem}
              activePage={currentPage}
              numPages={numPages}
              onPageClick={() => onChange(pageItem)}
            />
          );
        })
      )}
      <Button
        aria-label="Next page"
        disabled={isLastPage}
        onClick={() => {
          if (!isLastPage) {
            onChange(currentPage + 1);
          }
        }}
      >
        <KeyboardArrowRight />
      </Button>
    </div>
  );
};
