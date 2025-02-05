import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Stack from "@mui/material/Stack";
import { Pill } from "components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import type { FC } from "react";
import { isUUID } from "utils/uuid";

interface PillListProps {
	roles: readonly string[];
}

export const IdpPillList: FC<PillListProps> = ({ roles }) => {
	return (
		<Stack direction="row" spacing={1}>
			{roles.length > 0 ? (
				<Pill css={isUUID(roles[0]) ? styles.errorPill : styles.pill}>
					{roles[0]}
				</Pill>
			) : (
				<p>None</p>
			)}

			{roles.length > 1 && <OverflowPill roles={roles.slice(1)} />}
		</Stack>
	);
};

interface OverflowPillProps {
	roles: string[];
}

const OverflowPill: FC<OverflowPillProps> = ({ roles }) => {
	const theme = useTheme();

	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<Pill
					css={{
						backgroundColor: theme.palette.background.paper,
						borderColor: theme.palette.divider,
					}}
					data-testid="overflow-pill"
				>
					+{roles.length} more
				</Pill>
			</PopoverTrigger>

			<PopoverContent
				disableRestoreFocus
				disableScrollLock
				css={{
					".MuiPaper-root": {
						display: "flex",
						flexFlow: "column wrap",
						columnGap: 8,
						rowGap: 12,
						padding: "12px 16px",
						alignContent: "space-around",
						minWidth: "auto",
						backgroundColor: theme.palette.background.default,
					},
				}}
				anchorOrigin={{
					vertical: -4,
					horizontal: "center",
				}}
				transformOrigin={{
					vertical: "bottom",
					horizontal: "center",
				}}
			>
				{roles.map((role) => (
					<Pill key={role} css={isUUID(role) ? styles.errorPill : styles.pill}>
						{role}
					</Pill>
				))}
			</PopoverContent>
		</Popover>
	);
};

const styles = {
	pill: (theme) => ({
		backgroundColor: theme.experimental.pillDefault.background,
		borderColor: theme.experimental.pillDefault.outline,
		color: theme.experimental.pillDefault.text,
		width: "fit-content",
	}),
	errorPill: (theme) => ({
		backgroundColor: theme.roles.error.background,
		borderColor: theme.roles.error.outline,
		color: theme.roles.error.text,
		width: "fit-content",
	}),
} satisfies Record<string, Interpolation<Theme>>;
