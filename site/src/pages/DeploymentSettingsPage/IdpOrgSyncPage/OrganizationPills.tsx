import { Pill } from "components/Pill/Pill";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { FC } from "react";
import { cn } from "utils/cn";
import { isUUID } from "utils/uuid";

interface OrganizationPillsProps {
	organizations: readonly string[];
}

export const OrganizationPills: FC<OrganizationPillsProps> = ({
	organizations,
}) => {
	const orgs = organizations.map((org) => ({
		name: org,
		isUUID: isUUID(org),
	}));

	return (
		<div className="flex flex-row gap-2">
			{orgs.length > 0 ? (
				<Pill
					className={cn(
						"border-none w-fit",
						orgs[0].isUUID ? "bg-surface-destructive" : "bg-surface-secondary",
					)}
				>
					{orgs[0].name}
				</Pill>
			) : (
				<p>None</p>
			)}

			{orgs.length > 1 && <OverflowPillList organizations={orgs.slice(1)} />}
		</div>
	);
};

interface OverflowPillProps {
	organizations: { name: string; isUUID: boolean }[];
}

const OverflowPillList: FC<OverflowPillProps> = ({ organizations }) => {
	return (
		<TooltipProvider>
			<Tooltip delayDuration={0}>
				<TooltipTrigger asChild>
					<Pill
						className="min-h-4 min-w-6 bg-surface-secondary border-none px-3 py-1"
						data-testid="overflow-pill"
					>
						+{organizations.length}
					</Pill>
				</TooltipTrigger>

				<TooltipContent className="px-4 py-3 border-surface-quaternary">
					<ul className="flex flex-col gap-2 list-none my-0 pl-0">
						{organizations.map((organization) => (
							<li key={organization.name}>
								<Pill
									className={cn(
										"border-none w-fit",
										organization.isUUID
											? "bg-surface-destructive"
											: "bg-surface-secondary",
									)}
								>
									{organization.name}
								</Pill>
							</li>
						))}
					</ul>
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
