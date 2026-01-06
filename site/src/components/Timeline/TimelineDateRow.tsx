import { css, useTheme } from "@emotion/react";
import { TableCell, TableRow } from "components/Table/Table";
import type { FC } from "react";
import { formatDate } from "utils/date";
import { createDisplayDate } from "./utils";

export interface TimelineDateRow {
	date: Date;
}

export const TimelineDateRow: FC<TimelineDateRow> = ({ date }) => {
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
					padding: "8px 32px !important",
					background: `${theme.palette.background.paper} !important`,
					fontSize: 12,
					position: "relative",
					color: theme.palette.text.secondary,
					textTransform: "capitalize",
				}}
				title={formatDate(date)}
			>
				{createDisplayDate(date)}
			</TableCell>
		</TableRow>
	);
};
