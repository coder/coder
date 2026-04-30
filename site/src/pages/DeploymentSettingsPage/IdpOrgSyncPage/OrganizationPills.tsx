import type { FC } from "react";
import { Pill } from "#/components/Pill/Pill";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { isUUID } from "#/utils/uuid";

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
				<Pill type={orgs[0].isUUID ? "error" : "muted"} className="w-fit">
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
		<Tooltip>
			<TooltipTrigger asChild>
				<Pill type="muted" className="w-fit" data-testid="overflow-pill">
					+{organizations.length}
				</Pill>
			</TooltipTrigger>

			<TooltipContent className="px-4 py-3 border-surface-quaternary">
				<ul className="flex flex-col gap-2 list-none my-0 pl-0">
					{organizations.map((organization) => (
						<li key={organization.name}>
							<Pill
								type={organization.isUUID ? "error" : "muted"}
								className="w-fit"
							>
								{organization.name}
							</Pill>
						</li>
					))}
				</ul>
			</TooltipContent>
		</Tooltip>
	);
};
