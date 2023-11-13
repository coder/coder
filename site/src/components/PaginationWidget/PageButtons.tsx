import { PropsWithChildren } from "react";
import Button from "@mui/material/Button";
import { useTheme } from "@emotion/react";

type NumberedPageButtonProps = {
  pageNumber: number;
  totalPages: number;

  onClick?: () => void;
  highlighted?: boolean;
  disabled?: boolean;
};

export function NumberedPageButton({
  pageNumber,
  totalPages,
  onClick,
  highlighted = false,
  disabled = false,
}: NumberedPageButtonProps) {
  return (
    <BasePageButton
      name="Page button"
      aria-label={getNumberedButtonLabel(pageNumber, totalPages, highlighted)}
      onClick={onClick}
      highlighted={highlighted}
      disabled={disabled}
    >
      {pageNumber}
    </BasePageButton>
  );
}

type PlaceholderPageButtonProps = PropsWithChildren<{
  pagesOmitted: number;
}>;

export function PlaceholderPageButton({
  pagesOmitted,
  children = <>&hellip;</>,
}: PlaceholderPageButtonProps) {
  return (
    <BasePageButton
      disabled
      name="Omitted pages"
      aria-label={`Omitting ${pagesOmitted} pages`}
    >
      {children}
    </BasePageButton>
  );
}

type BasePageButtonProps = PropsWithChildren<{
  name: string;
  "aria-label": string;

  onClick?: () => void;
  highlighted?: boolean;
  disabled?: boolean;
}>;

function BasePageButton({
  children,
  onClick,
  name,
  "aria-label": ariaLabel,
  highlighted = false,
  disabled = false,
}: BasePageButtonProps) {
  const theme = useTheme();

  return (
    <Button
      css={
        highlighted && {
          borderColor: `${theme.palette.info.main}`,
          backgroundColor: `${theme.palette.info.dark}`,
        }
      }
      aria-label={ariaLabel}
      name={name}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

function getNumberedButtonLabel(
  page: number,
  totalPages: number,
  highlighted: boolean,
): string {
  if (highlighted) {
    return "Current Page";
  }

  if (page === 1) {
    return "First Page";
  }

  if (page === totalPages) {
    return "Last Page";
  }

  return `Page ${page}`;
}
