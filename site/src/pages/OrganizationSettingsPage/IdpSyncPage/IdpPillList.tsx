import type { Interpolation, Theme } from "@emotion/react";
import Stack from "@mui/material/Stack";
import { Pill } from "components/Pill/Pill";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
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
	return (
		<TooltipProvider>
			<Tooltip delayDuration={0}>
				<TooltipTrigger asChild>
					<Pill data-testid="overflow-pill">+{roles.length} more</Pill>
				</TooltipTrigger>

				<TooltipContent className="px-4 py-3 border-surface-quaternary">
					<ul className="flex flex-col gap-2 list-none my-0 pl-0">
						{roles.map((role) => (
							<li key={role}>
								<Pill css={isUUID(role) ? styles.errorPill : styles.pill}>
									{role}
								</Pill>
							</li>
						))}
					</ul>
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
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
