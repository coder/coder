import { makeStyles } from "@mui/styles";
import { useClickable, UseClickableResult } from "./useClickable";

interface UseClickableTableRowResult extends UseClickableResult {
  className: string;
  hover: true;
}

export const useClickableTableRow = (
  onClick: () => void,
): UseClickableTableRowResult => {
  const styles = useStyles();
  const clickable = useClickable(onClick);

  return {
    ...clickable,
    className: styles.row,
    hover: true,
  };
};

const useStyles = makeStyles((theme) => ({
  row: {
    cursor: "pointer",

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
      outlineOffset: -1,
    },

    "&:last-of-type": {
      borderBottomLeftRadius: theme.shape.borderRadius,
      borderBottomRightRadius: theme.shape.borderRadius,
    },
  },
}));
