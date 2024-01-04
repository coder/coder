import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import { type FC } from "react";
import { css, useTheme } from "@emotion/react";

export interface TimelineDateRow {
  date: Date;
  displayDate: string;
}

export const TimelineDateRow: FC<TimelineDateRow> = ({ date, displayDate }) => {
  const theme = useTheme();

  return (
    <TableRow
      css={css`
        &:not(:first-of-type) td {
          border-top: 1px solid ${theme.palette.divider};
        }
      `}
    >
      <TableCell
        css={{
          padding: `8px 32px !important`,
          background: `${theme.palette.background.paper} !important`,
          fontSize: 12,
          position: "relative",
          color: theme.palette.text.secondary,
          textTransform: "capitalize",
        }}
        title={date.toLocaleDateString()}
      >
        {displayDate}
      </TableCell>
    </TableRow>
  );
};
