import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
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
		<div className="flex flex-row gap-2">
			{roles.length > 0 ? (
				<Badge
					className="w-fit"
					variant={isUUID(roles[0]) ? "destructive" : "default"}
				>
					{roles[0]}
				</Badge>
			) : (
				<p>None</p>
			)}

			{roles.length > 1 && <OverflowPill roles={roles.slice(1)} />}
		</div>
	);
};

interface OverflowPillProps {
	roles: string[];
}

const OverflowPill: FC<OverflowPillProps> = ({ roles }) => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge className="w-fit" data-testid="overflow-pill">
					+{roles.length} more
				</Badge>
			</TooltipTrigger>

			<TooltipContent className="px-4 py-3 border-surface-quaternary">
				<ul className="flex flex-col gap-2 list-none my-0 pl-0">
					{roles.map((role) => (
						<li key={role}>
							<Badge
								className="w-fit"
								variant={isUUID(role) ? "destructive" : "default"}
							>
								{role}
							</Badge>
						</li>
					))}
				</ul>
			</TooltipContent>
		</Tooltip>
	);
};
