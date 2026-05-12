import Stack from "@mui/material/Stack";
import type { FC } from "react";
import { Pill } from "#/components/Pill/Pill";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { isUUID } from "#/utils/uuid";

interface PillListProps {
	roles: readonly string[];
}

export const IdpPillList: FC<PillListProps> = ({ roles }) => {
	return (
		<Stack direction="row" spacing={1}>
			{roles.length > 0 ? (
				<Pill className="w-fit" type={isUUID(roles[0]) ? "error" : "muted"}>
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
		<Tooltip>
			<TooltipTrigger asChild>
				<Pill type="muted" className="w-fit" data-testid="overflow-pill">
					+{roles.length} more
				</Pill>
			</TooltipTrigger>

			<TooltipContent className="px-4 py-3 border-surface-quaternary">
				<ul className="flex flex-col gap-2 list-none my-0 pl-0">
					{roles.map((role) => (
						<li key={role}>
							<Pill className="w-fit" type={isUUID(role) ? "error" : "muted"}>
								{role}
							</Pill>
						</li>
					))}
				</ul>
			</TooltipContent>
		</Tooltip>
	);
};
