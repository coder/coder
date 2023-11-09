import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import { type FC } from "react";
import { css, useTheme } from "@emotion/react";
import { createDisplayDate } from "./utils";

export interface TimelineDateRow {
  date: Date;
}

export const TimelineDateRow: FC<TimelineDateRow> = ({ date }) => {
  const theme = useTheme();

  return (
    <TableRow
      css={css`
        background: ${theme.deprecated.palette.background.paper};

        &:not(:first-of-type) td {
          border-top: 1px solid ${theme.deprecated.palette.divider};
        }
      `}
    >
      <TableCell
        css={{
          padding: `8px 32px !important`,
          background: `${theme.deprecated.palette.background.paperLight} !important`,
          fontSize: 12,
          position: "relative",
          color: theme.deprecated.palette.text.secondary,
          textTransform: "capitalize",
        }}
        title={date.toLocaleDateString()}
      >
        {createDisplayDate(date)}
      </TableCell>
    </TableRow>
  );
};
