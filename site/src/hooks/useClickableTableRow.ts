import { useTheme, type CSSObject } from "@emotion/react";
import { type MouseEventHandler } from "react";
import { type TableRowProps } from "@mui/material/TableRow";
import { useClickable, type UseClickableResult } from "./useClickable";

type UseClickableTableRowResult = UseClickableResult<HTMLTableRowElement> &
  TableRowProps & {
    css: CSSObject;
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
  const clickableProps = useClickable(onClick);
  const theme = useTheme();

  return {
    ...clickableProps,
    css: {
      cursor: "pointer",

      "&:focus": {
        outline: `1px solid ${theme.palette.secondary.dark}`,
        outlineOffset: -1,
      },

      "&:last-of-type": {
        borderBottomLeftRadius: 8,
        borderBottomRightRadius: 8,
      },
    },
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
