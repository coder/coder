import { useTheme } from "@emotion/react";
import { Pill } from "components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import type { FC } from "react";
import { cn } from "utils/cn";

interface PillListProps {
	organizations: readonly string[];
}

// used to check if the organization is a UUID
const UUID =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export const PillList: FC<PillListProps> = ({ organizations }) => {
	return (
		<div className="flex flex-row gap-2">
			{organizations.length > 0 ? (
				<Pill
					className={cn("border-none w-fit", {
						"bg-surface-error": UUID.test(organizations[0]),
						"bg-surface-secondary": !UUID.test(organizations[0]),
					})}
				>
					{organizations[0]}
				</Pill>
			) : (
				<p>None</p>
			)}

			{organizations.length > 1 && (
				<OverflowPill organizations={organizations.slice(1)} />
			)}
		</div>
	);
};

interface OverflowPillProps {
	organizations: string[];
}

const OverflowPill: FC<OverflowPillProps> = ({ organizations }) => {
	const theme = useTheme();

	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<Pill
					className="min-h-4 min-w-6 bg-surface-secondary border-none px-3 py-1"
					data-testid="overflow-pill"
				>
					+{organizations.length}
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
				{organizations.map((organization) => (
					<Pill
						key={organization}
						className={cn("border-none w-fit", {
							"bg-surface-error": UUID.test(organization),
							"bg-surface-secondary": !UUID.test(organization),
						})}
					>
						{organization}
					</Pill>
				))}
			</PopoverContent>
		</Popover>
	);
};
