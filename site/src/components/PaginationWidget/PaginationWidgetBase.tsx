import useMediaQuery from "@mui/material/useMediaQuery";
import { useTheme } from "@emotion/react";

import { PlaceholderPageButton, NumberedPageButton } from "./PageButtons";
import { buildPagedList } from "./utils";
import { PaginationNavButton } from "./PaginationNavButton";
import KeyboardArrowLeft from "@mui/icons-material/KeyboardArrowLeft";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";

export type PaginationWidgetBaseProps = {
  currentPage: number;
  pageSize: number;
  totalRecords: number;
  onPageChange: (newPage: number) => void;
};

export const PaginationWidgetBase = ({
  currentPage,
  pageSize,
  totalRecords,
  onPageChange,
}: PaginationWidgetBaseProps) => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const totalPages = Math.ceil(totalRecords / pageSize);

  if (totalPages < 2) {
    return null;
  }

  const onFirstPage = currentPage <= 1;
  const onLastPage = currentPage >= totalPages;

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
      <PaginationNavButton
        disabledMessage="You are already on the first page"
        disabled={onFirstPage}
        aria-label="Previous page"
        onClick={() => {
          if (!onFirstPage) {
            onPageChange(currentPage - 1);
          }
        }}
      >
        <KeyboardArrowLeft />
      </PaginationNavButton>

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
          onChange={onPageChange}
        />
      )}

      <PaginationNavButton
        disabledMessage="You're already on the last page"
        disabled={onLastPage}
        aria-label="Next page"
        onClick={() => {
          if (!onLastPage) {
            onPageChange(currentPage + 1);
          }
        }}
      >
        <KeyboardArrowRight />
      </PaginationNavButton>
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
