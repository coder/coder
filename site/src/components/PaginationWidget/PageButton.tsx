import Button from "@mui/material/Button";
import { makeStyles } from "@mui/styles";

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
  const styles = useStyles();
  return (
    <Button
      className={
        activePage === page
          ? `${styles.pageButton} ${styles.activePageButton}`
          : styles.pageButton
      }
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

const useStyles = makeStyles((theme) => ({
  pageButton: {
    "&:not(:last-of-type)": {
      marginRight: theme.spacing(0.5),
    },
  },

  activePageButton: {
    borderColor: `${theme.palette.info.main}`,
    backgroundColor: `${theme.palette.info.dark}`,
  },
}));
