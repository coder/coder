import { type MouseEventHandler } from "react";
import { type TableRowProps } from "@mui/material/TableRow";
import { makeStyles } from "@mui/styles";
import { useClickable, type UseClickableResult } from "./useClickable";

type UseClickableTableRowResult = UseClickableResult<HTMLTableRowElement> &
  TableRowProps & {
    className: string;
    hover: true;
    onAuxClick: MouseEventHandler<HTMLTableRowElement>;
  };

// Awkward type definition (the hover preview in VS Code isn't great, either),
// but this basically takes all click props from TableRowProps, but makes
// onClick required, and adds an optional onMiddleClick
type UseClickableTableRowConfig = {
  [Key in keyof TableRowProps as Key extends `on${string}Click`
    ? Key
    : never]: UseClickableTableRowResult[Key];
} & {
  onClick: MouseEventHandler<HTMLTableRowElement>;
  onMiddleClick?: MouseEventHandler<HTMLTableRowElement>;
};

export const useClickableTableRow = ({
  onClick,
  onAuxClick: externalOnAuxClick,
  onDoubleClick,
  onMiddleClick,
}: UseClickableTableRowConfig): UseClickableTableRowResult => {
  const styles = useStyles();
  const clickableProps = useClickable(onClick);

  return {
    ...clickableProps,
    className: styles.row,
    hover: true,
    onDoubleClick,
    onAuxClick: (event) => {
      const isMiddleMouseButton = event.button === 1;
      if (isMiddleMouseButton) {
        onMiddleClick?.(event);
      }

      externalOnAuxClick?.(event);
    },
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
