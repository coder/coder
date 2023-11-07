import useMediaQuery from "@mui/material/useMediaQuery";
import { useTheme } from "@emotion/react";

import { PlaceholderPageButton, NumberedPageButton } from "./PageButtons";
import { buildPagedList } from "./utils";
import { LeftNavButton, RightNavButton } from "./PaginationNavButtons";

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
}: PaginationWidgetBaseProps) => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const totalPages = Math.ceil(count / limit);

  if (totalPages < 2) {
    return null;
  }

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
      <LeftNavButton currentPage={currentPage} onChange={onChange} />

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

      <RightNavButton currentPage={currentPage} onChange={onChange} />
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
