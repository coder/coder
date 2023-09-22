import { type TableRowProps } from "@mui/material/TableRow";
import { makeStyles } from "@mui/styles";
import { useClickable, type UseClickableResult } from "./useClickable";

type UseClickableTableRowResult = UseClickableResult<HTMLTableRowElement> &
  TableRowProps & {
    className: string;
    hover: true;
  };

type TableRowOnClickProps = {
  [Key in keyof UseClickableTableRowResult as Key extends `on${string}Click`
    ? Key
    : never]: UseClickableTableRowResult[Key];
};

export const useClickableTableRow = ({
  onClick,
  ...optionalOnClickProps
}: TableRowOnClickProps): UseClickableTableRowResult => {
  const styles = useStyles();
  const clickableProps = useClickable<HTMLTableRowElement>(onClick);

  return {
    ...clickableProps,
    ...optionalOnClickProps,
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
