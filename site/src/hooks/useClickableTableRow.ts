/**
 * @file 2024-02-19 - MES - Sadly, even though this hook aims to make elements
 * more accessible, it's doing the opposite right now. Per axe audits, the
 * current implementation will create a bunch of critical-level accessibility
 * violations:
 *
 * 1. Nesting interactive elements (e.g., workspace table rows having checkboxes
 *    inside them)
 * 2. Overriding the native element's role (in this case, turning a native table
 *    row into a button, which means that screen readers lose the ability to
 *    announce the row's data as part of a larger table)
 *
 * It might not make sense to test this hook until the underlying design
 * problems are fixed.
 */
import { type CSSObject, useTheme } from "@emotion/react";
import type { TableRowProps } from "@mui/material/TableRow";
import type { MouseEventHandler } from "react";
import {
  type ClickableAriaRole,
  type UseClickableResult,
  useClickable,
} from "./useClickable";

type UseClickableTableRowResult<
  TRole extends ClickableAriaRole = ClickableAriaRole,
> = UseClickableResult<HTMLTableRowElement, TRole> &
  TableRowProps & {
    css: CSSObject;
    hover: true;
    onAuxClick: MouseEventHandler<HTMLTableRowElement>;
  };

// Awkward type definition (the hover preview in VS Code isn't great, either),
// but this basically extracts all click props from TableRowProps, but makes
// onClick required, and adds additional optional props (notably onMiddleClick)
type UseClickableTableRowConfig<TRole extends ClickableAriaRole> = {
  [Key in keyof TableRowProps as Key extends `on${string}Click`
    ? Key
    : never]: UseClickableTableRowResult<TRole>[Key];
} & {
  role?: TRole;
  onClick: MouseEventHandler<HTMLTableRowElement>;
  onMiddleClick?: MouseEventHandler<HTMLTableRowElement>;
};

export const useClickableTableRow = <
  TRole extends ClickableAriaRole = ClickableAriaRole,
>({
  role,
  onClick,
  onDoubleClick,
  onMiddleClick,
  onAuxClick: externalOnAuxClick,
}: UseClickableTableRowConfig<TRole>): UseClickableTableRowResult<TRole> => {
  const clickableProps = useClickable(onClick, (role ?? "button") as TRole);
  const theme = useTheme();

  return {
    ...clickableProps,
    css: {
      cursor: "pointer",

      "&:focus": {
        outline: `1px solid ${theme.palette.primary.main}`,
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
      // Regardless of which callback gets called, the hook won't stop the event
      // from bubbling further up the DOM
      const isMiddleMouseButton = event.button === 1;
      if (isMiddleMouseButton) {
        onMiddleClick?.(event);
      } else {
        externalOnAuxClick?.(event);
      }
    },
  };
};
