import { type ReactElement } from "react";
import Button from "@mui/material/Button";
import useMediaQuery from "@mui/material/useMediaQuery";
import KeyboardArrowLeft from "@mui/icons-material/KeyboardArrowLeft";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import { useTheme } from "@emotion/react";
import { PlaceholderPageButton, NumberedPageButton } from "./PageButton";
import { buildPagedList } from "./utils";

export type PaginationWidgetBaseProps = {
  count: number;
  page: number;
  limit: number;
  onChange: (newPage: number) => void;
};

export const PaginationWidgetBase = ({
  count,
  limit,
  onChange,
  page: currentPage,
}: PaginationWidgetBaseProps): ReactElement | null => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const totalPages = Math.ceil(count / limit);

  if (totalPages < 2) {
    return null;
  }

  const isFirstPage = currentPage <= 1;
  const isLastPage = currentPage >= totalPages;

  return (
    <div
      css={{
        justifyContent: "center",
        alignItems: "center",
        display: "flex",
        flexDirection: "row",
        padding: "20px",
        columnGap: "6px",
      }}
    >
      <Button
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
        <NumberedPageButton
          highlighted
          pageNumber={currentPage}
          totalPages={totalPages}
        />
      ) : (
        <PaginationRow
          currentPage={currentPage}
          totalPages={totalPages}
          onChange={onChange}
        />
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

type PaginationRowProps = {
  currentPage: number;
  totalPages: number;
  onChange: (newPage: number) => void;
};

function PaginationRow({
  currentPage,
  totalPages,
  onChange,
}: PaginationRowProps) {
  const pageInfo = buildPagedList(totalPages, currentPage);
  const pagesOmitted = totalPages - pageInfo.length - 1;

  return (
    <>
      {pageInfo.map((pageEntry) => {
        if (pageEntry === "left" || pageEntry === "right") {
          return (
            <PlaceholderPageButton
              key={pageEntry}
              pagesOmitted={pagesOmitted}
            />
          );
        }

        return (
          <NumberedPageButton
            key={pageEntry}
            pageNumber={pageEntry}
            totalPages={totalPages}
            highlighted={pageEntry === currentPage}
            onClick={() => onChange(pageEntry)}
          />
        );
      })}
    </>
  );
}
