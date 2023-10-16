import Button from "@mui/material/Button";
import { css, useTheme } from "@emotion/react";

interface PageButtonProps {
  activePage?: number;
  page?: number;
  placeholder?: string;
  numPages?: number;
  onPageClick?: (page: number) => void;
  disabled?: boolean;
}

export const PageButton = ({
  activePage,
  page,
  placeholder = "...",
  numPages,
  onPageClick,
  disabled = false,
}: PageButtonProps): JSX.Element => {
  const theme = useTheme();
  return (
    <Button
      css={[
        css`
          &:not(:last-of-type) {
            margin-right: ${theme.spacing(0.5)};
          }
        `,
        activePage === page && {
          borderColor: `${theme.palette.info.main}`,
          backgroundColor: `${theme.palette.info.dark}`,
        },
      ]}
      aria-label={`${page === activePage ? "Current Page" : ""} ${
        page === numPages ? "Last Page" : ""
      } Page${page}`}
      name={page === undefined ? undefined : "Page button"}
      onClick={() => onPageClick && page && onPageClick(page)}
      disabled={disabled}
    >
      <div>{page ?? placeholder}</div>
    </Button>
  );
};
